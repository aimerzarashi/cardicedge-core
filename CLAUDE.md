# コマンド

- `make up` / `make down` / `make migrate` / `make migrate-hash` / `make gen-orm` / `make gen-api` / `make fmt` / `make lint` / `make test`
- lint は `make lint`（gofmt の検出、go vet、staticcheck、カスタムリンター `go run ./tools/linter ./...`、scripts/check-migrations.sh）でファイルを書き換えない。フォーマットの修正は `make fmt`。staticcheck は go.mod の tool で管理する
- テストは `make test`（コンテナ内で `-race` 付き）で実行する。ホストには cgo がなく `-race` が使えない
- 単体で動かすときは `docker compose exec core go test ./<pkg> -race -run <Test>`

# ディレクトリ構成

| パス | 責務 |
|---|---|
| cmd/app | エントリポイント。HTTP サーバーの起動とグレースフルシャットダウン |
| cmd/app/cms_api.go | cms 用 GraphQL の部品を作り（インフラの実装をアプリ層・uif 層に渡す）、初期オペレーターの登録と入口の取り付けを分けて提供する。順番は run（main.go）で読む |
| internal/cms/uif/graphql | cms 用 GraphQL（`*.graphqls`・gqlgen.yml・リゾルバー・handler（/cms/graphql。JWT の認証））。generated.go・model は生成コード |
| internal/cms/app/authentication | JWT の (iss, sub) に紐づいたオペレーターを context に載せる（認証ミドルウェアが載せ、認可が読む） |
| internal/cms/app/command | コマンド（オペレーターの登録・初期オペレーターの登録・初めてのログインでの紐づけ） |
| internal/domain | 集約の共通の契約（Event・ID・EventStore 等）と、集約（`operator` 等）。`<集約>test` はテストで値の型を作る道具 |
| internal/eventstore・internal/identifier・internal/constraint | イベントの保存と復元、ID の発行、一意性の制約（domain_unique_keys）。`constrainttest` はテストで key から集約を引く・消す道具 |
| internal/domaindef | domain_entity_types・domain_event_types の対応。起動時に読み込み、集約の宣言と照合する |
| internal/platform/rdb | PostgreSQL への接続、トランザクション（`Transact`）。`rdbtest` はテストの道具 |
| internal/platform/idp | JWT の検証（発行元ごと）。`idptest` はテストで JWKS・JWT を作る道具 |
| internal/platform/config | 環境変数（すべて必須） |
| internal/platform/orm/sqlboiler | sqlboiler の設定と生成したモデル（`make gen-orm`） |
| migrations | DB マイグレーション（`NNNN_name.sql` + `atlas.sum`）。Atlas で適用する |
| local/migrate | マイグレーション用イメージ（Atlas Community + migrations。`atlas.hcl` で DATABASE_URL を読む） |
| scripts/check-migrations.sh | Atlas が検出しない migrations の違反（番号の重複、適用済みファイルの変更、適用済みより小さい番号の追加、指示行の後の空行）の検査 |
| tools/linter | カスタムリンター（検査の内容は README。対象の設定は main.go） |
| local/core | 開発用 Dockerfile（air によるホットリロード。uid 1000 の dev ユーザーで動く） |
| .build | 本番用 Dockerfile（distroless。compose.test.yaml でも使う） |

# 実装上の約束

- シャットダウン時間は `shutdownTimeout`（cmd/app） < air の `--build.kill_delay`（compose.yaml） < compose の `stop_grace_period` の順に保つ
- core は共有ネットワーク `diecardomain` に参加する（compose.yaml）。core-db は参加させない
- compose.test.yaml は game / cms が include する前提。サービス名・ポート・DB 接続情報を変える場合は利用側への影響を確認する
- distroless にはシェルが無いため、healthcheck は `/app healthcheck`（cmd/app/healthcheck.go）で行う
- マイグレーションは追加のみ。適用済みのファイルは書き換えない・消さない・改名しない。down は作らない（atlas.sum を作り直すと Atlas は検出しないため、check-migrations.sh で検査する）
- ファイル名は `NNNN_name.sql`（番号は数字のみ）。`-- atlas:txmode none` 等の指示行の直後には空行を入れる
- 新しいマイグレーションは、その時点の最大の番号 + 1 で作り、`make migrate-hash` で atlas.sum を作り直して SQL と一緒にコミットする（番号の抜け・atlas.sum の食い違いは Atlas がエラーにする）
- Atlas のバージョンは local/migrate/Dockerfile の FROM と .gitlab-ci.yml の ATLAS_VERSION を揃える（cms / game とも揃える）

# テストとカスタムリンター

- `internal` の値の型は、ほかのパッケージではテストでも生成関数（`NewXxx`）で作る（factory_enforcer）。テストでエラーを毎回確かめるのが冗長なら、テスト用の小さな関数を用意する
- testify にメッセージを付けない。テストでは `t.Context()` を使う
- 1 つの振る舞いは 1 つのテスト関数（`Test<対象>_<振る舞い>`）に書く。サブテストはテーブル駆動のときだけ使い、名前は英小文字のスネークケースの識別子（例: `blank_subject`）にし、その行の条件を表す。「正常系/」「異常系/」のような分類や `case1` は使わない（理由は README）
- 生成コードは、先頭の `Code generated ... DO NOT EDIT.` でリンターが自動で除く（除外の設定は要らない）。手で書くファイルにこの印を付けない

# イベントストアと GraphQL

- 集約を足したら、種別とイベント種別のマイグレーションを足し、`make gen-orm` し、起動時の `registry.Verify`（cmd/app）と domaindef のテストにその集約の `Types()` を足す
- 種別名・イベント名・スキーマの版は型が宣言する（リフレクションで推測しない）。集約の種別名は `domain.EntityType`、イベントの種別名は `domain.EventName`、一意な key の種別は `domain.UniqueKeyType` の値の型にし、各集約が生成関数で作る変数として宣言する（string のままだと名前どうしを取り違えてもコンパイルが通るため）。種別の id も `domaindef.EntityTypeID`・`EventTypeID` で分ける。payload の形を変えるときは版を上げ、古い版の golden（testdata/events）を残す
- `Decide` に時刻を渡す（中で time.Now を呼ばない）。`Evolve` は状態を書き換えない
- /cms/graphql はすべての呼び出しで JWT を確かめ、iss・aud で発行元を区別する（入口のパスやネットワークの位置を信じない）。誰の操作かは入力ではなく JWT の (iss, sub) だけで決める
- 層の分担: 認証（JWT の検証とオペレーターの特定・紐づけ）は uif のミドルウェア、認可は ui 層（resolver）、登録・紐づけは app 層のコマンド。初期オペレーターは cmd/app が app 層のコマンドを直接呼んで登録する
- GraphQL のスキーマを変えたら `make gen-api`。生成コード（generated.go・model）は手で直さない。操作を足すと `*.resolvers.go` に雛形（`panic("not implemented")`）ができ、消すとファイルの末尾に「!!! WARNING !!!」つきで古い実装が移されるので、片付ける
- スキーマの型・フィールド・引数・入力の項目には、必ず GraphQL の説明（`"..."`）を付ける（playground・型の生成で使う人に意味を伝えるため）。設計の理由は `#` のコメントに書き、説明と分ける
- 画面の文言は core で作らない。エラー型は `__typename` と値で理由を表し、`message` は開発者向けの短い英語の説明にする（言い回しは cms が作る）
- スキーマは common.graphqls（Query・Mutation・共通の型）と機能ごとのファイル（`extend type`）に分ける。操作の結果は `<操作>Result = <操作>Success | エラー型` の union にする（未認証は union に含めず 401）
- uif は app 層の型だけを使い、ドメインの型（internal/domain）を import しない。app 層のコマンドは素の値を受け取り、app 層の結果の型とエラー（`InputErrors`・`Err...`）を返す。GraphQL の型（uif/graphql/model）にも依存させない
- 操作を足すときは、resolver の最初に `requireOperator` で認可し、違えば `ForbiddenError` を返す。app 層の `InputErrors` は `ValidationErrors`（理由ごとの `FieldError` の型）に、app 層のエラーはそれぞれのエラー型に変え、それ以外だけを GraphQL の errors にする。上限などの値は uif に書き写さない
- 一意性の制約は、1 つの表 domain_unique_keys（key の種別の中で key が一意）で持つ。何を一意にするかはドメインが宣言する: key の種別の名前（`UniqueKeyTypes()`。domain_unique_key_types と起動時に照合する）と、key の作り方（`EmailKey`・`SubjectKey`・`card.NameKey` 等。正規化・複数の値のつなぎ方はここだけで決める）。制約を足すときは、key の種別のマイグレーション（domain_unique_key_types への INSERT）と、ドメインの宣言、domaindef のテストへの追加だけでよい
- 確保の関数の型（`UpdateXxxFunc`）はドメインに置き、実装は internal/constraint の `Update...` 関数（共通の updateUniqueKey を呼ぶだけ）。参照の api と同じく登録にも変更にも使え、ほかの集約が同じ key を使っていれば `WHERE NOT EXISTS` で INSERT 自体を起こさず、行が返らないことで重複を知る（ふだんの重複で DB に ERROR のログを出さない。まれな同時の競合による一意制約違反も重複として返す）。sqlboiler の Insert・Upsert は使わない（理由は constraint/constraint.go）
- 種類の違う集約で共有する key の種別（カード名）は、共有の実装（constraint/card.go。外に出さない）に、種類ごとの関数（constraint/creature.go 等）をかぶせ、ID の型は種類ごとの関数で守る
- テストで一意性の key から集約を引く・消すときは constrainttest を使う（テストに一意性の表の作りを書かない）
- イベントストアの版の衝突も、一意制約違反のエラーではなく `ON CONFLICT DO NOTHING RETURNING` で行が返らないことで知る
