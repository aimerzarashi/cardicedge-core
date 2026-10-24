# コマンド

- `make up` / `make down` / `make migrate` / `make migrate-hash` / `make gen-orm` / `make gen-api` / `make seed-master` / `make publish-master` / `make fmt` / `make lint` / `make test`
- lint は `make lint`（gofmt の検出、go vet、staticcheck、カスタムリンター `go run ./tools/linter ./...`、scripts/check-gen-api.sh、scripts/check-migrations.sh）でファイルを書き換えない。フォーマットの修正は `make fmt`。staticcheck は go.mod の tool で管理する
- テストは `make test`（テンプレートの DB を用意してから、コンテナ内で `-race` 付き）で実行する。ホストには cgo がなく `-race` が使えない
- 単体で動かすときは `docker compose exec core go test ./<pkg> -race -run <Test>`

# ディレクトリ構成

| パス | 責務 |
|---|---|
| cmd/app | エントリポイント。プロセスとしての起動だけ（設定・DB を開く・JWT の検証の準備・シグナル・HTTP の Listen・Serve とグレースフルシャットダウン・healthcheck・schema）。組み立ては internal/wiring |
| internal/wiring | 部品を組み立てて動かす（studio 用・game 用の DB ごとに種別と購読者を合わせる・照合・初期オペレーター・Hub の起動・HTTP の入口）。購読者の id と名前・どのサービスのものかの一覧（subscribers.go）もここに置く。cmd/app と結合テストの両方が使い、本番と同じ組み立てをテストで通す |
| internal/service/studio | studio のサービスの組み立て（studio.go。下の層の部品（コマンド・問い合わせ・リゾルバー・入口）をつなぎ、studio の read model の投影を登録する） |
| internal/service/studio/presentation/graphql | studio 用 GraphQL（`*.graphqls`・gqlgen.yml・リゾルバー・handler（/studio/graphql。JWT の認証）・データローダー（dataloader.go。リクエストごとに作る））。generated.go・model/models_gen.go は生成コード。model のほかのファイルは、gqlgen.yml で生成させずに使わせる手書きの型（`PresetDeck`・`StarterDeck`・カードと枚数の `DeckCardCount`。resolver に渡す値を持たせるため）。テストは tests/integration |
| internal/service/studio/application/authentication | JWT の (iss, sub) に紐づいたオペレーターを context に載せる（認証ミドルウェアが載せ、認可が読む） |
| internal/service/studio/application/command | コマンド（集約ごとの `XxxHandler`。オペレーターの登録・初期オペレーターの登録・初めてのログインでの紐づけ・Creature・Item・Spell の登録・プリセットデッキの登録・名前の変更・枚数の変更・削除・スターターデッキセットの設定） |
| internal/service/studio/application/input | 入力の誤り（理由ごとの型と、その一覧 `input.Errors`）。コマンドと問い合わせで共有し、presentation 層が GraphQL の `ValidationErrors` に写す |
| internal/service/studio/application/query | 問い合わせ（read model ごとの `XxxHandler`。入力を確かめ、reader で読み、ページ分けする） |
| internal/service/studio/application/query/readmodel | read model の状態の型（表の 1 行。`Card`・`PresetDeck`・`StarterDeckSet`）と、イベントから状態を作る `Evolve`・`IsTerminal` |
| internal/service/studio/infrastructure/repository | read model の DB の読み書き（read model ごとの `XxxWriter`（投影が使う `Upsert`・`Delete`）と `XxxReader`（`List`・`Get` 等）） |
| internal/service/game | game のサービスの組み立て（game.go。下の層の部品（コマンド・問い合わせ・リゾルバー・入口）をつなぎ、ポリシーと read model の投影を登録する）。層は studio と同じにする（core#34） |
| internal/service/game/presentation/graphql | game 用 GraphQL（`*.graphqls`・gqlgen.yml・リゾルバー・handler（/game/graphql。JWT の認証と、(iss, sub) のプレイヤーの特定））。generated.go・model/models_gen.go は生成コード。テストは tests/integration |
| internal/service/game/application/authentication | JWT の (iss, sub) と、登録済みならそのプレイヤーを context に載せる（認証ミドルウェアが載せ、コマンドが読む） |
| internal/service/game/application/command | コマンド（集約ごとの `XxxHandler`。プレイヤーの登録・自分のプレイヤーの取得・認証での (iss, sub) の特定・スターターの配布（始める・配る・済んだことの記録）・所持カードとデッキの作成）。配布のハンドラーは、配布の流れ（プロセスマネージャー）のイベントを受け取る入口 `Handle` も持ち、game.go が eventdriven のポリシーとして登録する（ポリシーを presentation 層に置くとドメインのイベントの型を import し、application 層に別のパッケージで置くとユースケースがユースケースを呼ぶ形になるため） |
| internal/service/game/application/master | マスタ（studio が決め、`make publish-master` で core_game に写したもの）を読む部品。スターターデッキセット（デッキの名前と、カードの公開 ID と枚数）の型と、読み出しの関数の型。studio の read model の型は使わず、配布に要る項目だけを持つ |
| internal/service/game/application/query | 問い合わせ（read model ごとの `XxxHandler`。`PlayerHandler` は `players` から配布の状態 `StarterGrantStatus` を返す。行が無い・列が NULL なら `IN_PROGRESS`） |
| internal/service/game/application/query/readmodel | read model の状態の型（`Player`・`PlayerStarterGrant`。どちらも `players` の 1 行の一部で、`Player` は player の投影が行を作り、`PlayerStarterGrant` は starter_grant の投影が配布の状態の列だけを書く）と、`Evolve`・`IsTerminal` |
| internal/service/game/infrastructure/repository | core_game の表の読み書き（マスタを読む `StarterDeckSetReader`（行が無ければ found が false）、read model の `PlayerWriter`・`PlayerStarterGrantWriter`（投影が使う `Upsert`・`Delete`。それぞれ自分の列だけを書き、行が無ければ作る）と `PlayerReader`） |
| internal/domain | 集約の共通の契約（Event・ID・EventStore 等）と、集約（`operator` 等）。`<集約>test` はテストで値の型を作る道具。game のスターターの配布は 3 つの集約でできている: `startergrant`（プレイヤーに 1 つ。配る中身の写しと、どこまで済んだかを持つプロセスマネージャー）・`collection`（プレイヤーの所持カード）・`deck`（1 デッキ 1 集約。デッキの編集とカードの取得が互いにぶつからないため）。どのカードも公開 ID で持つ（game の DB にカードの集約が無いため） |
| internal/eventstore・internal/identifier・internal/constraint | イベントの保存と復元（版と revision の流れは eventstore の README）、ID の発行と削除・種類を決めない公開 ID の引き方（`EntityRefResolver`）、制約（一意性 domain_unique_keys は unique_key.go、集約どうしの参照 domain_references は reference.go）。`constrainttest` はテストで key から集約を引き、参照を読む道具 |
| internal/domaindef | domain_entity_types・domain_event_types・domain_unique_key_types・domain_reference_types の id と名前の一覧（catalog.go の `Types`）と、それを DB に合わせる `Sync`。起動時に合わせて読み込み、ドメインの宣言と照合する |
| internal/eventdriven | イベントを受けて投影（Projection）とポリシー（Policy。イベントを 1 つずつ受けてコマンドを動かす。失敗の記録・間隔・上限・デッドレター）を動かす仕組み（Listener・Hub・Poller・チェックポイント・やることの表・失敗の記録）。構成は README |
| internal/platform/pagination | 前向きのカーソルによるページ分け（first・after）の部品 |
| internal/platform/rdb | PostgreSQL への接続、トランザクション（`Transact`）。`rdbtest` はテストの道具（`TEST_DB_DSN` のサーバーだけを使う。`Tx` は共有の core_test で最後に rollback する。`NewDatabase` はテンプレート core_template から自分だけの DB を複製する。core_test に commit する道具は出さない） |
| internal/platform/idp | JWT の検証（発行元ごと）。`idptest` はテストで JWKS・JWT を作る道具 |
| internal/platform/config | 環境変数（すべて必須） |
| internal/platform/orm/sqlboiler | sqlboiler の設定と生成したモデル（`make gen-orm`） |
| migrations | DB マイグレーション（`NNNN_name.sql` + `atlas.sum`）。Atlas で適用する |
| local/migrate | マイグレーション用イメージ（Atlas Community + migrations。`atlas.hcl` で DATABASE_URL を読む） |
| scripts/check-gen-api.sh | gqlgen の生成し忘れの検査（複製した木で `gqlgen generate` をやり直し、差分が出ないこと）。生成し忘れると、埋め込まれて動くスキーマ（`app schema` の出力を含む）と生成した実行のコードがずれるため |
| scripts/check-migrations.sh | Atlas が検出しない migrations の違反（番号の重複、適用済みファイルの変更、適用済みより小さい番号の追加、指示行の後の空行）の検査 |
| tools/linter | カスタムリンター（検査の内容は README。対象の設定は main.go） |
| tools/createdb | DB が無ければ作るコマンド（make test・CI が core_test とテンプレート core_template を作るのに使う） |
| tools/synctypes | DB に種別の一覧（`domaindef.Types`）を合わせるコマンド（make migrate・make test・CI が、core を起動しないで使う DB に入れる。どの DB かは `-env` で環境変数の名前を渡す） |
| tools/seedmaster | 開発で触る見本のマスタ（sample_master.json。カード・プリセットデッキ・スターターデッキセット）を、初期オペレーターとして studio のコマンドで登録するコマンド（`make seed-master`。設定済みなら何もしない）。core_game に公開するのは `make publish-master` |
| local/core-db | 開発・テストの core-db の初期化（core_game を作る。core_studio は `POSTGRES_DB` で作る） |
| tests/integration | 結合テスト（入口ごとに studio・game）。internal/wiring で組み立てたアプリをテストと同じプロセスで動かし、HTTP で呼ぶ（GraphQL の契約・認証・認可から、投影されて一覧に出るまで）。複製した DB を 1 つ共有して並列に流し、アプリが ERROR のログを出したら実行全体を失敗にする |
| tests/testhelper | 結合テストの道具（アプリの起動 `StartApp`（studio・game の入口の両方を持つ本番と同じ組み立て）・GraphQL の呼び出し・投影を待つ `Eventually`・オペレーターの用意・ERROR のログの記録） |
| local/core | 開発用 Dockerfile（air によるホットリロード。uid 1000 の dev ユーザーで動く） |
| .build | 本番用 Dockerfile（distroless。compose.test.yaml でも使う） |

- internal/service/<利用者> は、利用者（studio・game）ごとに縦に切った塊（core#29）。直下のパッケージ（studio.go・game.go）が組み立て、その下に層を置く。サービスの間で似た部品（入口の認証等）があっても、共通の部品にしない（認証の後の扱いがサービスごとに違うため。core#34）。層はレイヤードアーキテクチャ（Evans）の呼び方にする:

| 層 | 役割 | 依存してよい先 |
|---|---|---|
| presentation | 外から呼ばれる側（GraphQL のスキーマ・リゾルバー・入口・認証のミドルウェア・認可） | application |
| application | 使い方（コマンド・問い合わせ・入力の誤り・read model の型）。外に求めること（読み出し等）を関数の型（ポート）で持つ | domain |
| infrastructure | 外を呼ぶ側（read model の DB の読み書き）。application のポートを実装する | application・domain |

  依存は内向き（presentation・infrastructure → application → domain）。application は presentation・infrastructure を import しない。ドメインが外に求めること（イベントの保存・識別子・制約）も、domain が interface・関数の型で持ち、internal/eventstore・identifier・constraint が実装する。層を残すのは、import の行からどの層に依存しているかを読めるようにするため（フラットにする案は採らなかった）
- tools の下は開発・CI の道具。本番のコード（cmd/app・internal）から import しない（本番のバイナリは cmd/app から import するものだけで作る）

# 実装上の約束

- シャットダウン時間は `shutdownTimeout`（cmd/app） < air の `--build.kill_delay`（compose.yaml） < compose の `stop_grace_period` の順に保つ
- core は共有ネットワーク `diecardomain` に参加する（compose.yaml）。core-db は参加させない
- compose.test.yaml は game が include する前提（studio は前提にする版のイメージを使う。ふだんはリリース済みの版、マイルストーンの途中は手元の core から作った local の版。studio#14・studio#23）。サービス名・ポート・DB 接続情報を変える場合は利用側への影響を確認する
- distroless にはシェルが無いため、healthcheck は `/app healthcheck`（cmd/app/healthcheck.go）で行う
- `/app schema <入口>`（cmd/app/schema.go）は、その入口の GraphQL のスキーマを SDL で標準出力に書く（設定・DB を使わない）。studio・game は、前提にする版のイメージから `docker run --rm diecardomain/core:<版> schema studio`（game は `schema game`）で取り出して型を生成する（core#22）。HTTP のイントロスペクションは有効にしない（すべての呼び出しで JWT を確かめる決まりに例外を作らないため）。入口を足したら、そのサービスのパッケージの `WriteSchema` を `schemaWriters` に足す（presentation の graphql パッケージはサービスどうしで名前が重なり、別名の import は禁止のため）。`make gen-api`・scripts/check-gen-api.sh・cmd/app のスキーマのテストにも足す
- core は studio 用・game 用の 2 つの DB（`STUDIO_DB_DSN`・`GAME_DB_DSN`）を使う。開発・テストは同じサーバーの別の DB（core_studio・core_game）だが、別のサーバーにしても設定だけで動くようにする。表は DB どうしで共有せず、ほかのサービスの値は使う側が read model で持つ（持ち方は後で決める。core#33）
- マイグレーションは表の作り（DDL）だけを書き、両方の DB に同じものを流す（サービスごとに分けて二重に保たない）。種別・購読者の行は入れない（コードの一覧から起動時に入れる）
- マイグレーションは追加のみ。適用済みのファイルは書き換えない・消さない・改名しない。down は作らない（atlas.sum を作り直すと Atlas は検出しないため、check-migrations.sh で検査する）
- ファイル名は `NNNN_name.sql`（番号は数字のみ）。`-- atlas:txmode none` 等の指示行の直後には空行を入れる
- 新しいマイグレーションは、その時点の最大の番号 + 1 で作り、`make migrate-hash` で atlas.sum を作り直して SQL と一緒にコミットする（番号の抜け・atlas.sum の食い違いは Atlas がエラーにする）
- リリース前に限り、並びを作り直してよい（適用済みの DB をすべて作り直すことになるため）。作り直したら migrations/BASELINE の 1 行目を書き換える（check-migrations.sh が比較元との比べ合わせを飛ばす。同じ並びの中では今までどおり止める。core#33）
- Atlas のバージョンは local/migrate/Dockerfile の FROM と .gitlab-ci.yml の ATLAS_VERSION を揃える（studio / game とも揃える）

# テストとカスタムリンター

- `internal` の値の型は、ほかのパッケージではテストでも生成関数（`NewXxx`）で作る（factory_enforcer）。テストでエラーを毎回確かめるのが冗長なら、テスト用の小さな関数を用意する
- testify にメッセージを付けない。テストでは `t.Context()` を使う
- 1 つの振る舞いは 1 つのテスト関数（`Test<対象>_<振る舞い>`）に書く。サブテストはテーブル駆動のときだけ使い、名前は英小文字のスネークケースの識別子（例: `blank_subject`）にし、その行の条件を表す。「正常系/」「異常系/」のような分類や `case1` は使わない（理由は README）
- GraphQL の操作（認証・認可・入力の誤りの写し方・結果の型）は tests/integration で確かめ、presentation 層の単体テストは置かない（同じことを 2 か所で確かめないため）。下の層（readmodel・query・repository・command）は、それぞれのパッケージで確かめる
- 結合テストは DB を共有して並列に流すため、作るもの（名前・メールアドレス等）に `testhelper.UniqueID` を付け、それで絞って確かめる。DB 全体の状態を前提にすること（オペレーターが 1 人もいない等）は、application 層のテストで複製した DB を使って確かめる
- テスト用の種別・購読者の id の範囲は持たない。コードの一覧に無い種別・購読者を DB に足す・食い違わせるテストは、テンプレートから複製した DB（`rdbtest.NewDatabase`）で動かす（共有の core_test に残さず、ほかのテストの照合も崩さないため。core#33）
- 同じパッケージのほかのテストファイルからも使うヘルパーは、`helper_test.go`（テスト関数を置かない）に置く（使う側から定義の場所がわかり、テストファイルを消してもほかが壊れないように）
- 本体のある関数・メソッドは複数行で書く（書き方をそろえるため。core#15）。リンターでは検査しない（崩れてきたら検査を足すかを考え直す）。1 行でよいもの: 本体が空の関数（マーカーのメソッド）、無名関数（その場で小さな関数を渡す書き方）、gqlgen が生成するルートの入口（`Mutation()`・`Query()`、resolver を持たせた型の入口（`PresetDeck()` 等）。生成のたびに雛形の 1 行に戻るため）
- 生成コードは、先頭の `Code generated ... DO NOT EDIT.` でリンターが自動で除く（除外の設定は要らない）。手で書くファイルにこの印を付けない

# イベントストアと GraphQL

- 言葉の使い分け: ID と種類は entity（domain_entities・domain_entity_types・`entity_type_id`・`entity_id`・`domain.EntityType`・`EntityRef`。集約の根と、公開する子の entity（伝票の明細等）を含む）。版とイベントの宣言は aggregate（`aggregate_version`・`domain.AggregateTypes`。イベントを持つのは集約の根だけ）。ID を発行する・引く・消す操作は identifier（`internal/identifier`・`domain.Identifier`）（core#27）
- read model の名前は、表すものの複数形の表（`cards`・`preset_decks`）を軸にそろえる: 投影の名前は表と同じ、型は表の単数形（`readmodel.Card`）、書くもの・読むものは `repository.CardWriter`・`CardReader`、問い合わせは `query.CardHandler`。コマンドは集約ごとの `command.XxxHandler`（パッケージ名と重ねない。core#27）
- read model の表は、鍵と、検索・絞り込み・並べ替えに使う値を個別の列にし、表示に使う値は `metadata`（JSONB）に持つ（同じ項目が両方に要れば両方に持つ。migrations/0008 の決まり）。1 つの表を複数の投影が書くとき（`players`）は、それぞれが `metadata` の自分の項目だけを重ねて書く（core#38）
- 種別（集約・イベント・一意な key・参照）の id と名前は、コードの一覧（internal/domaindef/catalog.go の `Types`）で決める。id は自動で振らない（DB ごと・起動の順番で変わりうるため）。一度決めた id と名前は変えない・消さない。振り方は catalog.go のコメント（集約は 1000 から 100 刻み、イベントは集約の id ＋ 1 から、key・参照は 1000 から順に）
- 起動時に、DB ごとのトランザクション（advisory lock で core どうしを順にする）で、一覧のうち DB に無い種別を足し、DB の行が一覧とちょうど一致するかを確かめる（`domaindef.Sync`）。続けて、ドメインの宣言と DB を両方向に照合する（`Verify...`）。食い違えば起動を止める（行は消さない。core#20・core#33）
- 集約を足したら、表のマイグレーションが要れば足して `make gen-orm` し、種別の一覧（`Types`）にその集約とイベントを足し、起動時の照合（internal/wiring の `syncTypes` の集約の一覧）と domaindef のテストの `aggregates()` にその集約の `Types()` を足す
- 種別名・イベント名・スキーマの版は型が宣言する（リフレクションで推測しない）。集約の種別名は `domain.EntityType`、イベントの種別名は `domain.EventName`、一意な key の種別は `domain.UniqueKeyType` の値の型にし、各集約が生成関数で作る変数として宣言する（string のままだと名前どうしを取り違えてもコンパイルが通るため）。種別の id も `domaindef.EntityTypeID`・`EventTypeID` で分ける。payload の形を変えるときは版を上げ、古い版の golden（testdata/events）を残す
- `Decide` に時刻を渡す（中で time.Now を呼ばない）。`Evolve` は状態を書き換えない
- /studio/graphql・/game/graphql はすべての呼び出しで JWT を確かめ、iss・aud で発行元を区別する（入口のパスやネットワークの位置を信じない。aud はどちらも diecardomain-core で、iss で分ける）。誰の操作かは入力ではなく JWT の (iss, sub) だけで決める
- 層の分担: 認証（JWT の検証とオペレーターの特定・紐づけ）は presentation 層のミドルウェア、認可は presentation 層（resolver）、登録・紐づけは application 層のコマンド。初期オペレーターは internal/wiring が application 層のコマンドを直接呼んで登録する
- GraphQL のスキーマを変えたら `make gen-api`（し忘れは `make lint`・CI の lint が検出する）。生成コード（generated.go・model）は手で直さない。操作を足すと `*.resolvers.go` に雛形（`panic("not implemented")`）ができ、消すとファイルの末尾に「!!! WARNING !!!」つきで古い実装が移されるので、片付ける
- スキーマの型・フィールド・引数・入力の項目には、必ず GraphQL の説明（`"..."`）を付ける（playground・型の生成で使う人に意味を伝えるため）。設計の理由は `#` のコメントに書き、説明と分ける
- 画面の文言は core で作らない。エラー型は `__typename` と値で理由を表し、`message` は開発者向けの短い英語の説明にする（言い回しは studio が作る）
- スキーマは common.graphqls（Query・Mutation・共通の型）と機能ごとのファイル（`extend type`）に分ける。操作の結果は `<操作>Result = 成功の型 | エラー型` の union にする（未認証は union に含めず 401）。成功の型は、ミューテーションは `<操作>Success` で包む（返すのは操作の結果（revision・id 等）で、対象そのものとは限らないため。包めば、返す値を足し引きしても形が変わらない）。クエリは問い合わせたものを入れる（1 件なら対象の型（`PresetDeck`・`Player` 等）、一覧なら 1 ページの `XxxConnection`（Relay の決まり。core#12））。包む型は作らない（対象そのものを返すため、包むと 1 段深くなるだけ。core#35）
- presentation 層は application 層の型だけを使い、ドメインの型（internal/domain）を import しない。application 層のコマンドは素の値を受け取り、application 層の結果の型とエラー（`input.Errors`・`Err...`）を返す。GraphQL の型（presentation/graphql/model）にも依存させない
- 操作を足すときは、resolver の最初に `requireOperator` で認可し、違えば `ForbiddenError` を返す。application 層の `input.Errors`（internal/service/studio/application/input。コマンドと問い合わせで共有する）は `ValidationErrors`（理由ごとの `FieldError` の型）に、application 層のエラーはそれぞれのエラー型に変え、それ以外だけを GraphQL の errors にする。上限などの値は presentation 層に書き写さない
- 一意性の制約は、1 つの表 domain_unique_keys（key の種別の中で key が一意）で持つ。何を一意にするかはドメインが宣言する: key の種別の名前（`UniqueKeyTypes()`。domain_unique_key_types と起動時に照合する）と、key の作り方（`EmailKey`・`SubjectKey`・`card.NameKey` 等。正規化・複数の値のつなぎ方はここだけで決める）。制約を足すときは、種別の一覧（`Types` の `UniqueKeyTypes`）と、ドメインの宣言、起動時の照合（`VerifyUniqueKeyTypes`）と domaindef のテストへの追加だけでよい
- 確保の関数の型（`UpdateXxxFunc`）はドメインに置き、実装は internal/constraint の `Update...` 関数（共通の updateUniqueKey を呼ぶだけ）。参照の api と同じく登録にも変更にも使え、ほかの集約が同じ key を使っていれば `WHERE NOT EXISTS` で INSERT 自体を起こさず、行が返らないことで重複を知る（ふだんの重複で DB に ERROR のログを出さない。まれな同時の競合による一意制約違反も重複として返す）。一意性の制約（とイベントストア）では sqlboiler の Insert・Upsert は使わない（理由は constraint/unique_key.go・eventstore/eventstore.go）。read model の書き込みのような単純な上書きと、主キー・`IN` で引くような単純な読み出しでは使う。キーセットのページ分け（行の比べ方）や `LIKE ... ESCAPE` の部分一致を使う一覧の読み出しは、sqlboiler の条件の部品に無く `qm.Where` に SQL の断片を書くだけになるため、SQL を直接書く（repository の `CardReader.List`）
- 種類の違う集約で共有する key の種別（カード名）は、共有の実装（constraint/card.go。外に出さない）に、種類ごとの関数（constraint/creature.go・item.go・spell.go 等）をかぶせ、ID の型は種類ごとの関数で守る
- テストで一意性の key から集約を引くとき・参照を読むときは constrainttest を使う（テストに制約の表の作りを書かない）
- 集約どうしの参照（参照されている間は消せない）は、1 つの表 domain_references で持つ（一意性と同じく、参照ごとに表を作らない。core#24）。参照する側を消せば参照も消え（ON DELETE CASCADE）、参照されている側の識別子は消せない（ON DELETE RESTRICT）。何が何を参照するか（参照の種別）は参照する側の集約が宣言する（`ReferenceTypes()`。domain_reference_types と起動時に照合する）
  - 参照を足し引きする関数の型（`UpdateXxxReferencesFunc`）は参照する側のドメインに置き、変えた後の一覧を受け取ってそろえる。実装は internal/constraint（共通の updateReferences を呼ぶだけ）。参照する側のコマンドと同じトランザクションで呼ぶ
  - 参照を足すときは、種別の一覧（`Types` の `ReferenceTypes`）、ドメインの宣言、wiring の `VerifyReferenceTypes` と domaindef のテストへの追加
  - 参照される集約を消すコマンドを作るときは、消す前に参照を確かめて「使われている」を結果の型で返す（確かめ忘れても DB が止めるが、利用者には内部のエラーとしか見えないため）
  - 問い合わせ（query・repository）からは domain_references を読まない（書く側が整合性を守るための表で、イベントから作り直せず、制約にしない関係は入っていないため）。参照されている関係を画面に出したいときは、参照する側の read model に持たせる（core#25）
- ほかの集約は内部 ID で持つが、それは整合性のための参照（消せない制約がある）の話。消えうる集約から写したことの記録（スターターデッキの写しの元のプリセットデッキ等）は、公開 ID で持つ（消えた後も投影が読めるように。内部 ID だと識別子が消えて公開 ID を引けない。core#25）
- ゲーム全体で 1 つの集約（スターターデッキセット等）は、最初の操作で識別子を発行し、「1 つだけ」を一意な key（値は決まった文字列。例: `starter_deck_set.singleton`）で守る。内部 ID も画面から受け取らず、この key の持ち主から引く。初めての操作が重なったら、後のほうは「先に変えられた」にする（未設定を見て操作したのに、もうあった状況のため。core#25）
- イベントストアの版の衝突も、一意制約違反のエラーではなく `ON CONFLICT DO NOTHING RETURNING` で行が返らないことで知る。版が続いていない（版 1 でも直前の版の次でもない）イベントも保存せず、`domain.ErrVersionNotConsecutive` を返す（コードの誤りでだけ起きるので、呼ぶ側は分岐しない。core#24）
- 画面で見た内容をもとに、既にあるものを変える・消す操作（プリセットデッキの名前・枚数の変更・削除等）は、同じ形にそろえる（core#24。流れの図は internal/eventstore/README.md）:
  - 入力に `revision`（画面が見ていた集約の版）を含め、型にも `revision` を持たせる。マスタの公開の版と取り違えないよう「版」と呼ばない（日本語では「更新の番号」）
  - application 層のコマンドが、Load の直後に今の版と比べ、違えば「先に変えられた」（`command.ErrRevisionConflict` → `RevisionConflictError`）を返す。試し直さない（後の変更が黙って勝つため）。保存のときの版の衝突（`domain.ErrVersionConflict`）も同じエラーにする。変わらない操作（同じ名前等）でも、revision が古ければ返す
  - 結果は対象の公開 ID と新しい revision だけにし、内容を返さない。対象を id で指さない操作（ゲーム全体で 1 つの集約の設定等）は id も返さない（どの操作も受け付けない識別子を見せないため。core#25）。画面は詳細を読み直し、詳細の revision を持つ（表示している内容と revision を同じ時点のものにそろえるため。投影が遅れているあいだは次の操作が「先に変えられた」になるが、ふだんの遅れは数ミリ秒）
  - 新しく作る操作（登録）と、画面を通らない操作（ログイン時の紐づけ・方針）には要らない
- 一覧の各要素からほかの read model の値を引くとき（プリセットデッキのカード名等）は、値を書き写さず、データローダー（presentation/graphql の dataloader.go）で引く（N+1 にしないため）。束ねて読む関数は、その read model の問い合わせ（query）と repository に置く。並べ方の決まりが要るなら query に置き（`query.CompareCards`）、presentation 層・studio に書き写さない
