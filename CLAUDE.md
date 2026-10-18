# コマンド

- `make up` / `make down` / `make migrate` / `make migrate-hash` / `make gen-orm` / `make gen-api` / `make fmt` / `make lint` / `make test`
- lint は `make lint`（gofmt の検出、go vet、staticcheck、カスタムリンター `go run ./tools/linter ./...`、scripts/check-gen-api.sh、scripts/check-migrations.sh）でファイルを書き換えない。フォーマットの修正は `make fmt`。staticcheck は go.mod の tool で管理する
- テストは `make test`（テンプレートの DB を用意してから、コンテナ内で `-race` 付き）で実行する。ホストには cgo がなく `-race` が使えない
- 単体で動かすときは `docker compose exec core go test ./<pkg> -race -run <Test>`

# ディレクトリ構成

| パス | 責務 |
|---|---|
| cmd/app | エントリポイント。プロセスとしての起動だけ（設定・DB を開く・JWT の検証の準備・シグナル・HTTP の Listen・Serve とグレースフルシャットダウン・healthcheck・schema）。組み立ては internal/wiring |
| internal/wiring | 部品を組み立てて動かす（照合・初期オペレーター・Hub の起動・HTTP の入口）。cmd/app と結合テストの両方が使い、本番と同じ組み立てをテストで通す |
| internal/wiring/studio | studio 用 GraphQL の部品（コマンド・問い合わせ・リゾルバー・入口）と、studio の read model の投影の登録 |
| internal/studio/uif/graphql | studio 用 GraphQL（`*.graphqls`・gqlgen.yml・リゾルバー・handler（/studio/graphql。JWT の認証））。generated.go・model は生成コード。テストは tests/integration |
| internal/studio/app/authentication | JWT の (iss, sub) に紐づいたオペレーターを context に載せる（認証ミドルウェアが載せ、認可が読む） |
| internal/studio/app/command | コマンド（オペレーターの登録・初期オペレーターの登録・初めてのログインでの紐づけ・Creature の登録） |
| internal/studio/app/input | 入力の誤り（理由ごとの型と、その一覧 `input.Errors`）。コマンドと問い合わせで共有し、uif が GraphQL の `ValidationErrors` に写す |
| internal/studio/app/query/readmodel | read model の状態の型と、イベントから状態を作る `Evolve`・`IsTerminal`（カードの一覧 `CardList` 等） |
| internal/studio/inf/repository | read model の DB の読み書き（投影が書く `Upsert...`・`Delete...`） |
| internal/domain | 集約の共通の契約（Event・ID・EventStore 等）と、集約（`operator` 等）。`<集約>test` はテストで値の型を作る道具 |
| internal/eventstore・internal/identifier・internal/constraint | イベントの保存と復元、ID の発行、一意性の制約（domain_unique_keys）。`constrainttest` はテストで key から集約を引く道具 |
| internal/domaindef | domain_entity_types・domain_event_types の対応。起動時に読み込み、集約の宣言と照合する |
| internal/eventdriven | イベントを受けて投影（Projection）を動かす仕組み（Listener・Hub・Poller・チェックポイント・やることの表）。構成は README |
| internal/platform/pagination | 前向きのカーソルによるページ分け（first・after）の部品 |
| internal/platform/rdb | PostgreSQL への接続、トランザクション（`Transact`）。`rdbtest` はテストの道具（`Tx` は開発用の DB で最後に rollback する。`NewDatabase` はテンプレート core_template から自分だけの DB を複製する。開発用の DB に commit する道具は出さない） |
| internal/platform/idp | JWT の検証（発行元ごと）。`idptest` はテストで JWKS・JWT を作る道具 |
| internal/platform/config | 環境変数（すべて必須） |
| internal/platform/orm/sqlboiler | sqlboiler の設定と生成したモデル（`make gen-orm`） |
| migrations | DB マイグレーション（`NNNN_name.sql` + `atlas.sum`）。Atlas で適用する |
| local/migrate | マイグレーション用イメージ（Atlas Community + migrations。`atlas.hcl` で DATABASE_URL を読む） |
| scripts/check-gen-api.sh | gqlgen の生成し忘れの検査（複製した木で `gqlgen generate` をやり直し、差分が出ないこと）。生成し忘れると、埋め込まれて動くスキーマ（`app schema` の出力を含む）と生成した実行のコードがずれるため |
| scripts/check-migrations.sh | Atlas が検出しない migrations の違反（番号の重複、適用済みファイルの変更、適用済みより小さい番号の追加、指示行の後の空行）の検査 |
| tools/linter | カスタムリンター（検査の内容は README。対象の設定は main.go） |
| tools/createdb | DB が無ければ作るコマンド（make test・CI がテンプレート core_template を作るのに使う） |
| tests/integration | 結合テスト。internal/wiring で組み立てたアプリをテストと同じプロセスで動かし、HTTP で呼ぶ（GraphQL の契約・認証・認可から、投影されて一覧に出るまで）。複製した DB を 1 つ共有して並列に流し、アプリが ERROR のログを出したら実行全体を失敗にする |
| tests/testhelper | 結合テストの道具（GraphQL の呼び出し・投影を待つ `Eventually`・オペレーターの用意・ERROR のログの記録） |
| local/core | 開発用 Dockerfile（air によるホットリロード。uid 1000 の dev ユーザーで動く） |
| .build | 本番用 Dockerfile（distroless。compose.test.yaml でも使う） |

- tools の下は開発・CI の道具。本番のコード（cmd/app・internal）から import しない（本番のバイナリは cmd/app から import するものだけで作る）

# 実装上の約束

- シャットダウン時間は `shutdownTimeout`（cmd/app） < air の `--build.kill_delay`（compose.yaml） < compose の `stop_grace_period` の順に保つ
- core は共有ネットワーク `diecardomain` に参加する（compose.yaml）。core-db は参加させない
- compose.test.yaml は game が include する前提（studio は前提にする版のリリース済みのイメージを使う。studio#14）。サービス名・ポート・DB 接続情報を変える場合は利用側への影響を確認する
- distroless にはシェルが無いため、healthcheck は `/app healthcheck`（cmd/app/healthcheck.go）で行う
- `/app schema <入口>`（cmd/app/schema.go）は、その入口の GraphQL のスキーマを SDL で標準出力に書く（設定・DB を使わない）。studio・game は、前提にする版のイメージから `docker run --rm diecardomain/core:<版> schema studio` で取り出して型を生成する（core#22）。HTTP のイントロスペクションは有効にしない（すべての呼び出しで JWT を確かめる決まりに例外を作らないため）。入口を足したら `schemaWriters` にも足す
- マイグレーションは追加のみ。適用済みのファイルは書き換えない・消さない・改名しない。down は作らない（atlas.sum を作り直すと Atlas は検出しないため、check-migrations.sh で検査する）
- ファイル名は `NNNN_name.sql`（番号は数字のみ）。`-- atlas:txmode none` 等の指示行の直後には空行を入れる
- 新しいマイグレーションは、その時点の最大の番号 + 1 で作り、`make migrate-hash` で atlas.sum を作り直して SQL と一緒にコミットする（番号の抜け・atlas.sum の食い違いは Atlas がエラーにする）
- Atlas のバージョンは local/migrate/Dockerfile の FROM と .gitlab-ci.yml の ATLAS_VERSION を揃える（studio / game とも揃える）

# テストとカスタムリンター

- `internal` の値の型は、ほかのパッケージではテストでも生成関数（`NewXxx`）で作る（factory_enforcer）。テストでエラーを毎回確かめるのが冗長なら、テスト用の小さな関数を用意する
- testify にメッセージを付けない。テストでは `t.Context()` を使う
- 1 つの振る舞いは 1 つのテスト関数（`Test<対象>_<振る舞い>`）に書く。サブテストはテーブル駆動のときだけ使い、名前は英小文字のスネークケースの識別子（例: `blank_subject`）にし、その行の条件を表す。「正常系/」「異常系/」のような分類や `case1` は使わない（理由は README）
- GraphQL の操作（認証・認可・入力の誤りの写し方・結果の型）は tests/integration で確かめ、uif の単体テストは置かない（同じことを 2 か所で確かめないため）。下の層（readmodel・query・repository・command）は、それぞれのパッケージで確かめる
- 結合テストは DB を共有して並列に流すため、作るもの（名前・メールアドレス等）に `testhelper.UniqueID` を付け、それで絞って確かめる。DB 全体の状態を前提にすること（オペレーターが 1 人もいない等）は、app 層のテストで複製した DB を使って確かめる
- 同じパッケージのほかのテストファイルからも使うヘルパーは、`helper_test.go`（テスト関数を置かない）に置く（使う側から定義の場所がわかり、テストファイルを消してもほかが壊れないように）
- 本体のある関数・メソッドは複数行で書く（書き方をそろえるため。core#15）。リンターでは検査しない（崩れてきたら検査を足すかを考え直す）。1 行でよいもの: 本体が空の関数（マーカーのメソッド）、無名関数（その場で小さな関数を渡す書き方）、gqlgen が生成するルートの入口（`Mutation()`・`Query()`。生成のたびに雛形の 1 行に戻るため）
- 生成コードは、先頭の `Code generated ... DO NOT EDIT.` でリンターが自動で除く（除外の設定は要らない）。手で書くファイルにこの印を付けない

# イベントストアと GraphQL

- 集約を足したら、種別とイベント種別のマイグレーションを足し、`make gen-orm` し、起動時の `registry.Verify`（internal/wiring）と domaindef のテストにその集約の `Types()` を足す
- 種別名・イベント名・スキーマの版は型が宣言する（リフレクションで推測しない）。集約の種別名は `domain.EntityType`、イベントの種別名は `domain.EventName`、一意な key の種別は `domain.UniqueKeyType` の値の型にし、各集約が生成関数で作る変数として宣言する（string のままだと名前どうしを取り違えてもコンパイルが通るため）。種別の id も `domaindef.EntityTypeID`・`EventTypeID` で分ける。payload の形を変えるときは版を上げ、古い版の golden（testdata/events）を残す
- `Decide` に時刻を渡す（中で time.Now を呼ばない）。`Evolve` は状態を書き換えない
- /studio/graphql はすべての呼び出しで JWT を確かめ、iss・aud で発行元を区別する（入口のパスやネットワークの位置を信じない）。誰の操作かは入力ではなく JWT の (iss, sub) だけで決める
- 層の分担: 認証（JWT の検証とオペレーターの特定・紐づけ）は uif のミドルウェア、認可は ui 層（resolver）、登録・紐づけは app 層のコマンド。初期オペレーターは internal/wiring が app 層のコマンドを直接呼んで登録する
- GraphQL のスキーマを変えたら `make gen-api`（し忘れは `make lint`・CI の lint が検出する）。生成コード（generated.go・model）は手で直さない。操作を足すと `*.resolvers.go` に雛形（`panic("not implemented")`）ができ、消すとファイルの末尾に「!!! WARNING !!!」つきで古い実装が移されるので、片付ける
- スキーマの型・フィールド・引数・入力の項目には、必ず GraphQL の説明（`"..."`）を付ける（playground・型の生成で使う人に意味を伝えるため）。設計の理由は `#` のコメントに書き、説明と分ける
- 画面の文言は core で作らない。エラー型は `__typename` と値で理由を表し、`message` は開発者向けの短い英語の説明にする（言い回しは studio が作る）
- スキーマは common.graphqls（Query・Mutation・共通の型）と機能ごとのファイル（`extend type`）に分ける。操作の結果は `<操作>Result = <操作>Success | エラー型` の union にする（未認証は union に含めず 401）
- uif は app 層の型だけを使い、ドメインの型（internal/domain）を import しない。app 層のコマンドは素の値を受け取り、app 層の結果の型とエラー（`input.Errors`・`Err...`）を返す。GraphQL の型（uif/graphql/model）にも依存させない
- 操作を足すときは、resolver の最初に `requireOperator` で認可し、違えば `ForbiddenError` を返す。app 層の `input.Errors`（internal/studio/app/input。コマンドと問い合わせで共有する）は `ValidationErrors`（理由ごとの `FieldError` の型）に、app 層のエラーはそれぞれのエラー型に変え、それ以外だけを GraphQL の errors にする。上限などの値は uif に書き写さない
- 一意性の制約は、1 つの表 domain_unique_keys（key の種別の中で key が一意）で持つ。何を一意にするかはドメインが宣言する: key の種別の名前（`UniqueKeyTypes()`。domain_unique_key_types と起動時に照合する）と、key の作り方（`EmailKey`・`SubjectKey`・`card.NameKey` 等。正規化・複数の値のつなぎ方はここだけで決める）。制約を足すときは、key の種別のマイグレーション（domain_unique_key_types への INSERT）と、ドメインの宣言、domaindef のテストへの追加だけでよい
- 確保の関数の型（`UpdateXxxFunc`）はドメインに置き、実装は internal/constraint の `Update...` 関数（共通の updateUniqueKey を呼ぶだけ）。参照の api と同じく登録にも変更にも使え、ほかの集約が同じ key を使っていれば `WHERE NOT EXISTS` で INSERT 自体を起こさず、行が返らないことで重複を知る（ふだんの重複で DB に ERROR のログを出さない。まれな同時の競合による一意制約違反も重複として返す）。一意性の制約（とイベントストア）では sqlboiler の Insert・Upsert は使わない（理由は constraint/constraint.go・eventstore/eventstore.go）。read model の書き込みのような単純な上書きでは使ってよい
- 種類の違う集約で共有する key の種別（カード名）は、共有の実装（constraint/card.go。外に出さない）に、種類ごとの関数（constraint/creature.go 等）をかぶせ、ID の型は種類ごとの関数で守る
- テストで一意性の key から集約を引くときは constrainttest を使う（テストに一意性の表の作りを書かない）
- イベントストアの版の衝突も、一意制約違反のエラーではなく `ON CONFLICT DO NOTHING RETURNING` で行が返らないことで知る
- 既存の集約を変えるコマンドを足すときは、版の衝突（`domain.ErrVersionConflict`）をどう扱うかを決める（自動で試し直すと後の変更が黙って勝つため、利用者に「先に変えられた」と返すか、読み直して試し直すかを操作ごとに選ぶ）。今は新しい集約を作るコマンドばかりで衝突が起きず、参照の api も試し直していない
