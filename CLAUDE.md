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
| internal/service/studio/application/command | コマンド（集約ごとの `XxxHandler`。オペレーターの登録・初期オペレーターの登録・初めてのログインでの紐づけ・Creature・Item・Spell の登録・カードの公開と使用不可（`CardHandler`。公開 ID で種類を引いて各集約に振り分ける）・プリセットデッキの登録・名前の変更・枚数の変更・削除・スターターデッキ（`StarterDeckHandler`）の登録（空から・プリセットデッキから写して）・名前と枚数の変更・削除・公開・使用不可・ボード（`BoardHandler`）の登録・名前とレイアウトと既定のルールの変更・削除・公開・使用不可（レイアウトの違反は `board.ValidateLayout` の結果を、既定のルールの違反は `board.NewDefaultRules` のエラーを、入力の項目のパスに写す。公開の条件を満たさなければ `BoardNotPublishableError`。保存せずに判定する `CheckLayout` も持つ。ボードを指さず、イベントを作らず、`ChangeLayout` と同じ `newLayout` で入力の誤りを写し、公開できない理由の一覧を返す。core#59）・ゲーム全体のルールの起動時の作成と読み出し（`GameRulesHandler`。読み出しの関数 `GetGameRulesFunc` を、プリセットデッキとスターターデッキのコマンドに渡す）・`studio_rules` の起動時の作成と読み出し（`StudioRulesHandler`。読み出しの関数 `GetStudioRulesFunc` を、ボードのコマンドに渡す）） |
| internal/service/studio/application/input | 入力の誤り（理由ごとの型と、その一覧 `input.Errors`）。コマンドと問い合わせで共有し、presentation 層が GraphQL の `ValidationErrors` に写す |
| internal/service/studio/application/query | 問い合わせ（read model ごとの `XxxHandler`。入力を確かめ、reader で読み、ページ分けする） |
| internal/service/studio/application/query/readmodel | read model の状態の型（表の 1 行。`Card`・`PresetDeck`・`StarterDeck`・`Board`・`GameRules`・`StudioRules`）と、イベントから状態を作る `Evolve`・`IsTerminal` |
| internal/service/studio/infrastructure/repository | read model の DB の読み書き（read model ごとの `XxxWriter`（投影が使う `Upsert`・`Delete`）と `XxxReader`（`List`・`Get` 等）） |
| internal/service/game | game のサービスの組み立て（game.go。下の層の部品（コマンド・問い合わせ・リゾルバー・入口）をつなぎ、ポリシーと read model の投影を登録する）。層は studio と同じにする（core#34） |
| internal/service/game/presentation/graphql | game 用 GraphQL（`*.graphqls`・gqlgen.yml・リゾルバー・handler（/game/graphql。JWT の認証と、(iss, sub) のプレイヤーの特定））。generated.go・model/models_gen.go は生成コード。テストは tests/integration |
| internal/service/game/application/authentication | JWT の (iss, sub) と、登録済みならそのプレイヤーを context に載せる（認証ミドルウェアが載せ、コマンドが読む） |
| internal/service/game/application/command | コマンド（集約ごとの `XxxHandler`。プレイヤーの登録・自分のプレイヤーの取得・認証での (iss, sub) の特定・スターターの配布（始める・配る・済んだことの記録）・所持カードとデッキの作成・アバターの作成（`AvatarHandler`。名前の誤り・同じプレイヤーの中での名前の重なり・上限・未登録を返す。上限は `LimitReachedError` で値を持つ））。配布のハンドラーは、配布の流れ（プロセスマネージャー）のイベントを受け取る入口 `Handle` も持ち、game.go が eventdriven のポリシーとして登録する（ポリシーを presentation 層に置くとドメインのイベントの型を import し、application 層に別のパッケージで置くとユースケースがユースケースを呼ぶ形になるため） |
| internal/service/game/application/input | 入力の誤り（`input.Errors` と理由ごとの型。理由は game で要るもの（`Required`・`TooLong`・`OutOfRange`・`InvalidFormat`）から足す）。サービスごとに縦に切る決まりなので、studio のものを import せず同じ形で置く。presentation 層が GraphQL の `ValidationErrors` に写す |
| internal/service/game/application/master | マスタ（studio が決め、`make publish-master` で core_game に写したもの）を読む部品。公開中のスターターデッキ（デッキの名前と、カードの公開 ID と枚数）の型と、それを決まった順（名前の正規化 → 公開 ID）で読む関数の型（1 つも無ければ空）。studio の read model の型は使わず、配布に要る項目だけを持つ |
| internal/service/game/application/rules | 全体のルール（運営が決める、プレイヤーによらない値）を読む部品。写した `game_rules` を読む関数の型 `GetRulesFunc`（行が無ければ found が false）と、上限の値を返す `GetQuotaLimitFunc`、それを作る `FromPublished`（写されていなければ `ErrNotPublished`。既定の値に黙って戻さない） |
| internal/service/game/application/query | 問い合わせ（read model ごとの `XxxHandler`。`PlayerHandler` は `players` から配布の状態 `StarterGrantStatus` を返す。行が無い・列が NULL なら `IN_PROGRESS`。`AvatarHandler` は `avatars` からプレイヤーのアバターを作った順に返す。無ければ空の一覧。`BoardHandler` は写した `boards` のうち公開中のボードを、名前の正規化 → 公開 ID の順にページ分けして返す（一覧と詳細。使用不可・準備中は見つからない扱い。型は studio の read model のものを使わず、盤面に要る項目だけ）） |
| internal/service/game/application/query/readmodel | read model の状態の型（`Player`・`PlayerStarterGrant`・`Avatar`（`avatars` の 1 行）。どちらも `players` の 1 行の一部で、`Player` は player の投影が行を作り、`PlayerStarterGrant` は starter_grant の投影が配布の状態の列だけを書く）と、`Evolve`・`IsTerminal` |
| internal/service/game/infrastructure/repository | core_game の表の読み書き（マスタを読む `StarterDeckReader`（公開中のスターターデッキだけ）・`GameRulesReader`（行が無ければ found が false）、read model の `PlayerWriter`・`PlayerStarterGrantWriter`（投影が使う `Upsert`・`Delete`。それぞれ自分の列だけを書き、行が無ければ作る）と `PlayerReader`、`avatars` の `AvatarWriter`・`AvatarReader`（プレイヤーで絞って作った順に読む）、マスタを読む `BoardReader`（公開中のボードだけ。一覧は行の比べ方でカーソルの続きを取り、詳細は公開中でなければ `query.ErrBoardNotFound`）） |
| internal/domain | 集約の共通の契約（Event・ID・EventStore・制約の型（`UniqueKey`・`ReferenceType`・`QuotaType`・`Quota`・`QuotaLimit`）・名前の比べ方 `NormalizeName`（name.go）等）と、集約（`operator` 等）。`<集約>test` はテストで値の型を作る道具。game のスターターの配布は 3 つの集約でできている: `startergrant`（プレイヤーに 1 つ。配る中身の写しと、どこまで済んだかを持つプロセスマネージャー）・`collection`（プレイヤーの所持カード）・`deck`（1 デッキ 1 集約。デッキの編集とカードの取得が互いにぶつからないため）。`gamerules`（ゲーム全体のルール。値オブジェクト `Rules` と既定の値 `Defaults()`、ゲーム全体で 1 つの集約）・`studiorules`（studio でマスタを作るときの決まり。同じ形）。どのカードも公開 ID で持つ（game の DB にカードの集約が無いため） |
| internal/eventstore・internal/identifier・internal/constraint | イベントの保存と復元（版と revision の流れは eventstore の README）、ID の発行と削除・種類を決めない公開 ID の引き方（`EntityRefResolver`）、制約（一意性 domain_unique_keys は unique_key.go、集約どうしの参照 domain_references は reference.go、数の上限 domain_quotas は quota.go）。`constrainttest` はテストで key から集約を引き、参照を読む道具 |
| internal/domaindef | domain_entity_types・domain_event_types・domain_unique_key_types・domain_reference_types・domain_quota_types の id と名前の一覧（catalog.go の `Types`）と、それを DB に合わせる `Sync`。起動時に合わせて読み込み、ドメインの宣言と照合する |
| internal/eventdriven | イベントを受けて投影（Projection）とポリシー（Policy。イベントを 1 つずつ受けてコマンドを動かす。失敗の記録・間隔・上限・デッドレター）を動かす仕組み（Listener・Hub・Poller・チェックポイント・やることの表・失敗の記録・複数台で単位を分け合うアドバイザリーロック）。構成は README |
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
| tools/seedmaster | 開発で触る見本のマスタ（sample_master.json。カード・プリセットデッキ・スターターデッキ・ボード）を、初期オペレーターとして studio のコマンドで登録するコマンド（`make seed-master`。同じ名前のものがあれば飛ばすので、何度流しても同じ結果になる）。core_game に公開するのは `make publish-master`。見本のカードは登録した後に公開中にする。スターターデッキはプリセットデッキから写して作り、公開する。ボードは登録してレイアウトと既定のルールを入れ、公開する（見本は 8×8 の周回で、拠点 1・チェックポイント 3・分かれ道 2 か所。既定のルールは登録の直後と違う値（目標の魔力 5000）で、変更の操作も流れる。土地は火・地・水・風・無・複の順に属性を付け、6 つがどれも出てくる。公開の条件は domain の関数で始める前に確かめる）。ゲーム全体のルールと studio_rules も、起動時と同じ関数で（無ければ）作る |
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
- core は共有ネットワーク `diecardomain-game`（game 用）と `diecardomain-studio`（studio 用）の両方に参加する（compose.yaml。作るのは make up）。core-db は参加させない。分けるのは、game から studio に内部から届く経路そのものを無くすため（core#45）
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
- 種別（集約・イベント・一意な key・参照）の id と名前は、コードの一覧（internal/domaindef/catalog.go の `Types`）で決める。id は自動で振らない（DB ごと・起動の順番で変わりうるため）。一度決めた id と名前は変えない・消さない（main に入る前（リリース前）は除く。core#42 でスターターデッキセットの種別を、開発の DB を作り直す前提で消した）。振り方は catalog.go のコメント（集約は 1000 から 100 刻み、イベントは集約の id ＋ 1 から、key・参照・上限は 1000 から順に）
- 起動時に、DB ごとのトランザクション（advisory lock で core どうしを順にする）で、一覧のうち DB に無い種別を足し、DB の行が一覧とちょうど一致するかを確かめる（`domaindef.Sync`）。続けて、ドメインの宣言と DB を両方向に照合する（`Verify...`）。食い違えば起動を止める（行は消さない。core#20・core#33）
- 集約を足したら、表のマイグレーションが要れば足して `make gen-orm` し、種別の一覧（`Types`）にその集約とイベントを足し、起動時の照合（internal/wiring の `syncTypes` の集約の一覧）と domaindef のテストの `aggregates()` にその集約の `Types()` を足す
- 種別名・イベント名・スキーマの版は型が宣言する（リフレクションで推測しない）。集約の種別名は `domain.EntityType`、イベントの種別名は `domain.EventName`、一意な key の種別は `domain.UniqueKeyType` の値の型にし、各集約が生成関数で作る変数として宣言する（string のままだと名前どうしを取り違えてもコンパイルが通るため）。種別の id も `domaindef.EntityTypeID`・`EventTypeID` で分ける。payload の形を変えるときは版を上げ、古い版の golden（testdata/events）を残す
- イベントの payload の項目で、ほかのものを指す項目の名前（core#46）。何の ID かは型（各集約の event.go）と domain_entities の通番（種類をまたいで重ならない）で分かるため、名前は表現をそろえるためのもの:
  - ほかの集約が 1 つだけ出てくる: 集約の名前（`player`・`starter_grant`・`card`）。`_id` は付けず、略さない
  - 同じ集約が 2 つ以上出てくる: 役割 ＋ 集約の名前（例 `giver_player`・`receiver_player`）。役割だけにしない（何を指すかが名前から読めなくなるため）
  - そのイベントの操作をした人: `<動作>_by`（`registered_by` 等）。操作した人と持ち主が同じなら付けず、集約の名前だけにする
  - 消えた後も残す参照（公開 ID で持つもの）: `<対象>_id`（`card_id`・`preset_deck_id`）
- Go のコードで、構造体に入れる前の値を持つ名前（ローカル変数・関数の引数・関数の型の引数）は、中身が分かる名前にする（ID なら `<集約>ID`、名前なら `<対象>Name` 等。例 `playerID player.ID`・`starterGrantID`）。集約の名前だけにすると、import しているパッケージ（`player` 等）と重なり、ID と集約そのものを取り違えやすいため（core#46）。構造体の項目は payload の JSON の名前に合わせる（`Player player.ID \`json:"player"\``・`StarterGrant startergrant.ID \`json:"starter_grant"\``。項目名と JSON の名前の間にずれを作らないため）。操作した人の `registeredBy` 等は役割の名前のまま（`By` で ID と読めるため）
- core を複数台で動かすとき、イベントの処理は処理の単位（購読者 × 集約）ごとにアドバイザリーロックで 1 台だけが行う（core#19。eventdriven の README の「複数台で動かす」）。ロックが外れた間は 2 台が同じ単位を処理しうるので、ポリシーのコマンドは同時に 2 回呼ばれても DB の結果が 1 回になるよう、一意な key・状態で守る。DB の外への副作用（メール等）はポリシーから直接起こさない（要るときは outbox を考える）
- `Decide` に時刻を渡す（中で time.Now を呼ばない）。`Evolve` は状態を書き換えない
- /studio/graphql・/game/graphql はすべての呼び出しで JWT を確かめ、iss・aud で発行元を区別する（入口のパスやネットワークの位置を信じない。aud はどちらも diecardomain-core で、iss で分ける）。誰の操作かは入力ではなく JWT の (iss, sub) だけで決める
- 層の分担: 認証（JWT の検証とオペレーターの特定・紐づけ）は presentation 層のミドルウェア、認可は presentation 層（resolver）、登録・紐づけは application 層のコマンド。初期オペレーターは internal/wiring が application 層のコマンドを直接呼んで登録する
- GraphQL のスキーマを変えたら `make gen-api`（し忘れは `make lint`・CI の lint が検出する）。生成コード（generated.go・model）は手で直さない。操作を足すと `*.resolvers.go` に雛形（`panic("not implemented")`）ができ、消すとファイルの末尾に「!!! WARNING !!!」つきで古い実装が移されるので、片付ける
- スキーマの型・フィールド・引数・入力の項目には、必ず GraphQL の説明（`"..."`）を付ける（playground・型の生成で使う人に意味を伝えるため）。設計の理由は `#` のコメントに書き、説明と分ける
- 画面の文言は core で作らない。エラー型は `__typename` と値で理由を表し、`message` は開発者向けの短い英語の説明にする（言い回しは studio が作る）
  - 上限に達したエラー（`<対象>LimitReachedError`）は、そのときの上限の値を `max` で返す（studio は上限の値を書き写さず、core が返した値で「N 個まで」の文言を作るため。入力の誤りの `FieldOutOfRange` の `max` と同じ考え。core#44）。application 層では `command.LimitReachedError`（理由の `Err...LimitReached` を包み、`errors.Is` で理由を、`errors.As` で値を取る）で返す
- スキーマは common.graphqls（Query・Mutation・共通の型）と機能ごとのファイル（`extend type`）に分ける。操作の結果は `<操作>Result = 成功の型 | エラー型` の union にする（未認証は union に含めず 401）。成功の型は、ミューテーションは `<操作>Success` で包む（返すのは操作の結果（revision・id 等）で、対象そのものとは限らないため。包めば、返す値を足し引きしても形が変わらない）。クエリは問い合わせたものを入れる（1 件なら対象の型（`PresetDeck`・`Player` 等）、一覧なら 1 ページの `XxxConnection`（Relay の決まり。core#12））。包む型は作らない（対象そのものを返すため、包むと 1 段深くなるだけ。core#35）
- presentation 層は application 層の型だけを使い、ドメインの型（internal/domain）を import しない。application 層のコマンドは素の値を受け取り、application 層の結果の型とエラー（`input.Errors`・`Err...`）を返す。GraphQL の型（presentation/graphql/model）にも依存させない
- 操作を足すときは、resolver の最初に `requireOperator` で認可し、違えば `ForbiddenError` を返す（studio）。game は、登録済みのプレイヤーが要る操作の resolver の最初に `requirePlayer`（presentation/graphql/authorization.go）で確かめ、未登録なら `PlayerNotRegisteredError` を返す（`registerPlayer`・`me` は未登録でも呼べる）。application 層の `input.Errors`（internal/service/studio/application/input。コマンドと問い合わせで共有する）は `ValidationErrors`（理由ごとの `FieldError` の型）に、application 層のエラーはそれぞれのエラー型に変え、それ以外だけを GraphQL の errors にする。上限などの値は presentation 層に書き写さない
- 集約をまたぐ制約の守り方の目安（設計メモ「集約をまたぐ制約の守り方」）: 1 つの集約の中で閉じる決まりは Decide で守る。集約をまたぐ単純な決まり（一意性・数・参照）で、対象がそれぞれ別々に変わるものは、DB の制約の表（domain_unique_keys・domain_quotas・domain_references）を既定にする（軽く、作った時点で必ず守れ、並列に強い）。表はイベントから作り直せない書く側の整合性のための記録になる。対象をまとめて 1 つの版に並べるまとめ役の集約は、対象のどの操作も 1 つの版に並んで並列に動けないため、別々に変わる対象には使わない
- 一意性の制約は、1 つの表 domain_unique_keys（key の種別の中で key が一意）で持つ。何を一意にするかはドメインが宣言する: key の種別の名前（`UniqueKeyTypes()`。domain_unique_key_types と起動時に照合する）と、key の作り方（`EmailKey`・`SubjectKey`・`card.NameKey`・`deck.NameKey` 等。名前の比べ方は `domain.NormalizeName` に 1 つだけ置く。正規化・複数の値のつなぎ方はここだけで決める）。制約を足すときは、種別の一覧（`Types` の `UniqueKeyTypes`）と、ドメインの宣言、起動時の照合（`VerifyUniqueKeyTypes`）と domaindef のテストへの追加だけでよい
- 確保の関数の型（`UpdateXxxFunc`）はドメインに置き、実装は internal/constraint の `Update...` 関数（共通の updateUniqueKey を呼ぶだけ）。参照の api と同じく登録にも変更にも使え、ほかの集約が同じ key を使っていれば `WHERE NOT EXISTS` で INSERT 自体を起こさず、行が返らないことで重複を知る（ふだんの重複で DB に ERROR のログを出さない。まれな同時の競合による一意制約違反も重複として返す）。一意性の制約（とイベントストア）では sqlboiler の Insert・Upsert は使わない（理由は constraint/unique_key.go・eventstore/eventstore.go）。read model の書き込みのような単純な上書きと、主キー・`IN` で引くような単純な読み出しでは使う。キーセットのページ分け（行の比べ方）や `LIKE ... ESCAPE` の部分一致を使う一覧の読み出しは、sqlboiler の条件の部品に無く `qm.Where` に SQL の断片を書くだけになるため、SQL を直接書く（repository の `CardReader.List`）
- 数の上限の制約は、1 つの表 domain_quotas（種別 × 持ち主の内部 ID ごとに使っている数 `used`）で持つ（一意性・参照と同じく、上限ごとに表を作らない。core#39）。何を数えるかはドメインが宣言する: 上限の種別（`QuotaTypes()`。`domain.QuotaType`。domain_quota_types と起動時に照合する）と、持ち主ごとの単位の作り方（`deck.PlayerQuota(owner)`）、確保と解放の関数の型（`ReserveQuotaFunc`・`ReleaseQuotaFunc`）、上限のエラー（`ErrLimitReached`）、既定の値は `gamerules.Defaults()`（ドメインの定数にはしない）
  - 確保は作るコマンドと同じトランザクションの 1 つの文で「上限未満なら 1 増やす」（constraint/quota.go の `reserveQuota`）。上限なら行が返らず、上限のエラーを返す（エラーにせず、トランザクションも中断しない）。同時に取り合っても DB が同じ行を順番に更新するので、流し直しは要らない。上限の値は DDL・CHECK に書かず文に渡す（下限の 0 以上だけ CHECK）。解放は `releaseQuota`（0 のときは減らさず、エラーにしない）。実装は constraint の `Reserve...Quota`・`Release...Quota`（共通の関数を呼ぶだけ）
  - 上限の値は、コマンドが game の application/rules の `GetQuotaLimitFunc`（写した `game_rules` を読む部品）から受け取る。ドメインの既定の値を直接読まない（値を変えられるようにしても、コマンドを変えないため。テストでは上限を小さくして確かめる）
  - 上限を足すときは、種別の一覧（`Types` の `QuotaTypes`。上限は 1000 から順に）、ドメインの宣言、wiring の `VerifyQuotaTypes`、domaindef のテスト（helper_test の `quotaTypes()`）への追加と、rules の `FromPublished` の種別と `Rules` の項目の対応
- カード（Creature・Item・Spell）の状態は、準備中（`draft`、既定）・公開中（`published`）・使用不可（`retired`）。値の型と移り方の決まりは `internal/domain/card`（`Status.Publish`・`Retire`）に 1 つだけ置き、3 つの集約が同じ形（`PublishedEvent`・`RetiredEvent`）で持つ。移れるのは 準備中 → 公開中・公開中 → 使用不可・使用不可 → 公開中。公開中にもう一度公開する・使用不可にもう一度使用不可にするのは変わらない操作でイベントを作らず成功する（revision が古ければ `RevisionConflictError`）。準備中 → 使用不可は `InvalidCardStatusTransitionError`、公開中 → 準備中はできない。操作は `publishCard`・`retireCard`（公開 ID で指し、`EntityRefResolver` で種類を引く）
  - プリセットデッキに新しく入れる・枚数を増やすカードは公開中だけ（`input.Unavailable` → `FieldUnavailable`）。すでに入っているカードは使用不可になっても残せる（減らす・外す・変えないは確かめない）。状態はコマンドがその集約をイベントストアで読んで確かめる（読むだけで書かない）。読んでから保存するまでの間に使用不可にされる競合は許す（プリセットデッキはカードの状態を守る集約ではなく、使用不可は新しく入れないための印のため）
  - `make publish-master` が game に写す game_rules は行をすべて、cards は公開中・使用不可のカードだけ（準備中は写さない。使用不可はプレイヤーが持っているので写す）。cards の `status` は絞り込みと写す選びに使うため個別の列、`metadata` にも status と revision を持つ
- ゲーム全体のルールは、1 つの集約 `game_rules`（studio のサービス。一意な key `game_rules.singleton`。core#43）が値オブジェクト `gamerules.Rules` で持つ。何の決まりかで項目を分け（`Player`・`Deck`（デッキに共通）・`StarterDeck`（スターターデッキだけ））、どのサービスで使うかでは分けない。カード名・デッキ名の長さは値の形の決まりなので入れない
  - 作るのは studio の起動時（internal/wiring。初期オペレーターと同じく、あれば何もしない。先を越されても成功）で、値は `Defaults()`。値の変更のコマンドは core#23 で作る
  - studio は集約をイベントストアで読み（`GameRulesHandler.Get`。read model を読まず、投影の遅れで古い値を見ない）、確かめてから保存するコマンドに `GetGameRulesFunc` で渡す。ドメインの `Decide`・`NewDeck` は既定の値を直接読まず、コマンドが読んだルールを受け取る（テストでは値を小さくして確かめる）
  - game は read model `game_rules` を `make publish-master` で core_game に写したものを、`GameRulesReader`（行が無ければ found が false）で読む。まだ写されていなければエラーにし、既定の値に黙って戻さない（配布のポリシーでは待てば直る失敗）
  - 値を足す手順: `Rules` に項目と既定の値を足す（検証は `New`）。state・read model（`metadata` の `rules`）・game の読み出しは `Rules` をそのまま持つので変えない。使う側のコマンドで読むだけ。イベントの payload の形が変わるときは版を上げ、古い版の golden も残す
- studio でマスタを作るときの決まりは、1 つの集約 `studio_rules`（studio のサービス。一意な key `studio_rules.singleton`。core#52）が値オブジェクト `studiorules.Rules` で持つ。`game_rules` と同じ形（何の決まりかで項目を分ける。今は `Board`（`MinSize`・`MaxSize`。幅・高さに共通）だけ）で、game では使わない（`make publish-master` で写さない。マイグレーションは両方の DB に流すので core_game にも空の表ができる）。game でも表示する値（カード名・デッキ名の長さ）は入れない（変えると game の画面に響くため）
  - 作るのは studio の起動時（`game_rules` と同じ。値は `Defaults()`）。値の変更のコマンドは core#23 で作る。ボードのコマンドは集約をイベントストアで読み（`StudioRulesHandler.Get`）、範囲の外を `FieldOutOfRange`（`min`・`max` は `studio_rules` の値）にする。ドメインの `Decide` は既定の値を直接読まず、`RegisterCommand`・`ChangeLayoutCommand` の `Rules` を使う
  - studio 用の GraphQL の `Query.studioRules` は read model `studio_rules`（購読者 1010）を読む。まだ投影されていなければ既定の値に黙って戻さず、エラーにする（`query.ErrStudioRulesNotProjected`。待てば直る失敗）
- スターターデッキは、1 デッキ 1 集約 `starter_deck`（studio のサービス。core#42）。空から作るか、プリセットデッキから写して作る（名前・カード・枚数を写し、写した元の公開 ID と revision を、写したときだけ持つ。元は参照にしない）。状態（準備中・公開中・使用不可）の型と移り方はカードと同じ決まり（`starterdeck.Status`。型は共有しない）。名前・枚数の変更と削除は準備中だけ（`StarterDeckNotDraftError`）。準備中は決まり（40 枚等）を満たさなくてよい
  - 名前は、状態によらずすべてのスターターデッキの中で一意（一意な key `starter_deck.name`。登録・名前の変更で確保し、削除の識別子の削除で外れる。公開・使用不可では触らない）。入れたカードは参照 `starter_deck.card` で持つ
  - 公開するとき（使用不可からの公開し直しも）確かめるのは、合計がちょうど `deck.total_card_count`（`Decide`）、入っているカードがすべて公開中（コマンドがイベントストアで読む）、公開中の数が `starter_deck.max_published` 未満（quota `starter_deck.published`。持ち主は `game_rules` の内部 ID を key `game_rules.singleton` から引く）。使用不可にすると quota を 1 戻す。公開中にもう一度公開する・使用不可にもう一度するのは、変わらない操作でイベントを作らず成功する
  - `make publish-master` は starter_decks を全行（状態つき）で game に写し、game は公開中だけを（名前の正規化 → 公開 ID の順に）読んで配る。配るデッキの数は公開中の数（1 以上。1 つも無ければ `ErrMasterNotPublished` で待つ）。配るデッキの名前はスターターデッキの名前
- アバターは、1 アバター 1 集約 `avatar`（game のサービス。core#40）。作る操作 `registerAvatar` と、自分のアバターを見る `Player.avatars`（作った順のリスト）だけで、名前の変更・削除は使う場面が出てから。名前は 20 文字まで（`avatar.MaxNameLength`。デッキの名前とは型を分けて変えられるようにする）で、1 人の中で一意（一意な key `avatar.name`＝持ち主 × `domain.NormalizeName` した名前。ほかのプレイヤーとは同じ名前でよい）
  - 数の上限は quota `avatar.per_player`（持ち主はプレイヤーの内部 ID）。値は `game_rules` の `player.max_avatars` で、`rules.FromPublished` が対応させる。上限に達したら `AvatarLimitReachedError`（`max` に上限の値。application 層は `command.LimitReachedError`）。写された `game_rules` が無ければ作らない（`rules.ErrNotPublished`）
  - 制約は 1 つのトランザクションの中で名前 → 上限の順に確保する（どれかが失敗すれば全体が戻る）。read model `avatars`（鍵 `avatar_id`、絞り込みと並べ替えに `player_id`・`registered_at`、表示は `metadata`）は投影 `avatars`（購読者 1008）が書き、持ち主の内部 ID は書くときに公開 ID に引き直す
- ボードは、1 ボード 1 集約 `board`（studio のサービス。core#48）。形は四角形の格子（マスの形の項目は持たない。六角形等を足すときに、形の無いボードを四角形として足す）、マスは座標（x が列、y が行。0 始まり）と種類（`land`・`base`・`checkpoint`）と属性（`board.Attribute`。火 `fire`・地 `earth`・水 `water`・風 `wind`・無 `neutral`・複 `multi` の 6 つ。土地は必ずどれか 1 つを持ち、拠点・チェックポイントは持たない。複は 4 色のマークが描かれた特殊な土地で組み合わせは持たない。カードの属性とは型を分け、効き方は対戦の決まりなので持たない。マスの JSON は土地でなければ `attribute` を省く。core#54）、つながりは向きの無い隣どうし（`board.Connection` は並べ方の小さい方を a にそろえる）。大きさの範囲は幅・高さに共通で `studio_rules` の `board.min_size`・`max_size`（既定は 1〜32。`board.NewSize` は 1 以上の値の形だけを確かめ、範囲は `RegisterCommand`・`ChangeLayoutCommand` の `Rules` で、コマンドを動かす側が `studio_rules` を読んで渡す。範囲は書くときにだけ確かめ、保存済みのイベントを読み直すときは確かめない）で、登録のイベントが大きさを明示して持つ（既定の 8×8 を変えても過去の意味が変わらないため。登録の直後は 8×8 を範囲に収めた値で、8 は `board.DefaultSize` のまま `studio_rules` に入れない）。状態の型と移り方・名前・準備中だけの変更と削除はスターターデッキと同じ（`board.Status`。型は共有しない）。対戦の既定のルール（`board.DefaultRules`。core#56）は値オブジェクトで、目標の魔力 `target_magic`（届いて拠点に戻ると勝ち。1 以上）・開始時の魔力 `starting_magic`・周回のボーナス `lap_bonus`（拠点に戻ったとき）・チェックポイントのボーナス `checkpoint_bonus`（通ったとき）（この 3 つは 0 以上）の 4 つの整数で、目標の魔力は開始時の魔力より大きい。上限は持たない。決まりは書くときの `board.NewDefaultRules`（違反は `errors.Join` でまとめて返し、`errors.Is` で項目ごとに分ける）に 1 つだけ置き、保存済みのイベントを読み直すときは確かめない（大きさの範囲と同じ）。登録のイベントが値を明示して持ち（登録の直後は定数 8000・600・300・100。変えても過去のボードは変わらない）、変更は別の操作 `changeBoardDefaultRules`（`ChangeDefaultRulesCommand`・`DefaultRulesChangedEvent`。変えた後の 4 つの値の全体で、差分にしない）で、準備中だけ。同じ値ならイベントを作らない。名前を「既定のルール」にしたのは、studio_rules の `BoardRules`（大きさの範囲）と紛らわしいため。入力の誤りは `input.TooSmall{Min}`（`FieldTooSmall`。上限の無い値の下限。上限が必須の `FieldOutOfRange` は使えない）と `input.NotGreaterThan{Than}`（`FieldNotGreaterThan`。目標の魔力 → `startingMagic`）
  - レイアウトの決まり（格子の中・1 座標に 1 マス・土地には属性があり拠点・チェックポイントには無い（理由 `attribute_required`・`attribute_not_allowed`。部分は `attribute`）・つながりの両端は置いてあるマスで隣どうし・同じつながりは 1 つ）は、準備中でも保存の時点で守り、`board.ValidateLayout`（違反を「tiles / connections の何番目か・x / y / a / b・理由」の一覧で返す）の 1 か所に置く。application 層はこの一覧を入力の項目のパス（`width`・`height`・`tiles.N.<種類>.x`・`tiles.N.<種類>.y`（`<種類>` は入力で書いた項目の名前 `land`・`base`・`checkpoint`）・`tiles.N`・`connections.N.a`・`connections.N.b`・`connections.N`）に写すだけで、決まりを 2 か所に書かない（つながりの a・b は入力で書いた側に戻す）。マスの GraphQL の型は種類ごとに分ける: 出力は interface `BoardTile`（`x`・`y`）を `LandTile`（`attribute` を持つ）・`BaseTile`・`CheckpointTile` が実装し（種類は `__typename` で見分ける）、入力 `BoardTileInput` は `@oneOf { land, base, checkpoint }`。土地に属性が無い・拠点に属性がある組み合わせは入力の形として起きないので、`attribute_required`・`attribute_not_allowed` の違反が application 層に届けば内部のエラー（コードの誤り）にする（ドメインの `ValidateLayout` の決まりは残す）。`@oneOf` の「ちょうど 1 つ」は、変数で送られた値だと gqlparser・gqlgen が確かめないので、presentation 層で確かめ、満たさなければ `tiles.N` の `FieldInvalidFormat`（core#55）
  - 公開するとき（使用不可からの公開し直しも）確かめるのは、拠点がちょうど 1 つ・チェックポイントが 1 つ以上・すべてのマスに拠点からつながりをたどって行けること（行き止まりは認める。`Decide`、`Layout.Readiness`）。満たさなければ `*board.NotPublishableError`（`Readiness`。拠点の座標・チェックポイントの数・行けないマス。行けないマスは拠点がちょうど 1 つのときだけ計算し、そうでなければ空）。満たさない理由の一覧は `Readiness.Problems()`（`PublishProblem`＝種類 `PublishProblemKind` ＋関わるマスの座標。順は 拠点の数 → チェックポイント → 行けないマス）の 1 か所で作り、理由を足すときは種類を足してここに並べる（core#58 が相手のいない転送円・橋を足す）。判定は `CheckReadiness`（置いてあるマスとつながりから。`Layout.Readiness` はこれを呼ぶ）で、投影も同じ関数を使う。GraphQL は interface `BoardPublishProblem` を理由ごとの型（`BaseCountProblem`・`NoCheckpointProblem`・`UnreachableTilesProblem`）に分け、`Board.publishProblems` と `BoardNotPublishableError.problems` が同じ一覧を返す（studio 用だけ。game には置かない。core#57）。保存せずに判定する `Query.checkBoardLayout`（core#59）は、`ChangeBoardLayoutInput` から id・revision を除いた入力を受け取り、入力の誤りは保存と同じ（`newLayout` を共有）、誤りが無ければ同じ `Readiness.Problems()` の一覧を `publishProblems` で返す。ボードの ID を受け取らないので、権限は特定のボードではなく、保存と同じ `requireOperator`（紐づいたオペレーターか）で確かめる
  - レイアウトの変更は変えた後の全体を受け取り、`Decide` が前と比べた差分（大きさ・置いたマス・消したマス・種類か属性を変えたマス・つないだ／切ったつながり）を 1 つのイベント `LayoutChangedEvent` にする。変わらなければイベントを作らない。マスを消して切れたつながりは切ったつながりに入る
  - 名前は、状態によらずすべてのボードの中で一意（一意な key `board.name`。登録・名前の変更で確保し、削除の識別子の削除で外れる）。公開中の数の上限は持たない。read model `boards`（購読者 1009）は、鍵と並べ替え・絞り込みの値（名前・状態）を列、マスとつながり（y → x、a → b の順）と既定のルール（`default_rules`）とマスの数（`tile_counts`。土地の合計・拠点・チェックポイント・属性ごとの土地の数。レイアウトが変わるたびに投影が置いてあるマス全体から数え直す。studio は core が返した一覧を画面で絞り込まない決まりなので、数は core が返す）と公開できない理由（`publish_problems`。`[{"kind": "base_count"|"no_checkpoint"|"unreachable_tiles", "positions": [{"x","y"}]}]` で、空なら `[]`。レイアウトが変わるたびに投影が `CheckReadiness` で計算し直し、読むときは計算しない。状態は見ず、公開中・使用不可のボードもレイアウトから計算した結果（公開中は条件を満たすので空）を返す。理由を足す前に投影した行には無く、読み出しは誤りにするので、投影を作り直す）を含む表示用の値を `metadata` に持つ。`make publish-master` は cards と同じ選び方（公開中・使用不可だけ。準備中は写さない）で core_game の `boards` に写し、game は公開中だけを読む（`Query.boards`・`Query.board`。使用不可は一覧にも詳細にも出さず、詳細は見つからない扱い。読むのは登録済みのプレイヤーだけ。`default_rules`・`tile_counts` が無い行は 0 の値にせず読み出しの誤りにする）
- 種類の違う集約で共有する key の種別（カード名）は、共有の実装（constraint/card.go。外に出さない）に、種類ごとの関数（constraint/creature.go・item.go・spell.go 等）をかぶせ、ID の型は種類ごとの関数で守る
- テストで一意性の key から集約を引くとき・参照を読むときは constrainttest を使う（テストに制約の表の作りを書かない）
- 集約どうしの参照（参照されている間は消せない）は、1 つの表 domain_references で持つ（一意性と同じく、参照ごとに表を作らない。core#24）。参照する側を消せば参照も消え（ON DELETE CASCADE）、参照されている側の識別子は消せない（ON DELETE RESTRICT）。何が何を参照するか（参照の種別）は参照する側の集約が宣言する（`ReferenceTypes()`。domain_reference_types と起動時に照合する）
  - 参照を足し引きする関数の型（`UpdateXxxReferencesFunc`）は参照する側のドメインに置き、変えた後の一覧を受け取ってそろえる。実装は internal/constraint（共通の updateReferences を呼ぶだけ）。参照する側のコマンドと同じトランザクションで呼ぶ
  - 参照を足すときは、種別の一覧（`Types` の `ReferenceTypes`）、ドメインの宣言、wiring の `VerifyReferenceTypes` と domaindef のテストへの追加
  - 参照される集約を消すコマンドを作るときは、消す前に参照を確かめて「使われている」を結果の型で返す（確かめ忘れても DB が止めるが、利用者には内部のエラーとしか見えないため）
  - 問い合わせ（query・repository）からは domain_references を読まない（書く側が整合性を守るための表で、イベントから作り直せず、制約にしない関係は入っていないため）。参照されている関係を画面に出したいときは、参照する側の read model に持たせる（core#25）
- ほかの集約は内部 ID で持つが、それは整合性のための参照（消せない制約がある）の話。消えうる集約から写したことの記録（スターターデッキの写しの元のプリセットデッキ等）は、公開 ID で持つ（消えた後も投影が読めるように。内部 ID だと識別子が消えて公開 ID を引けない。core#25）
- ゲーム全体で 1 つの集約（`game_rules` 等）は、最初の操作で識別子を発行し、「1 つだけ」を一意な key（値は決まった文字列。例: `game_rules.singleton`）で守る。内部 ID も画面から受け取らず、この key の持ち主から引く。初めての操作が重なったら、後のほうは「先に変えられた」にする（未設定を見て操作したのに、もうあった状況のため。core#25）
- イベントストアの版の衝突も、一意制約違反のエラーではなく `ON CONFLICT DO NOTHING RETURNING` で行が返らないことで知る。版が続いていない（版 1 でも直前の版の次でもない）イベントも保存せず、`domain.ErrVersionNotConsecutive` を返す（コードの誤りでだけ起きるので、呼ぶ側は分岐しない。core#24）
- 画面で見た内容をもとに、既にあるものを変える・消す操作（プリセットデッキの名前・枚数の変更・削除等）は、同じ形にそろえる（core#24。流れの図は internal/eventstore/README.md）:
  - 入力に `revision`（画面が見ていた集約の版）を含め、型にも `revision` を持たせる。マスタの公開の版と取り違えないよう「版」と呼ばない（日本語では「更新の番号」）
  - application 層のコマンドが、Load の直後に今の版と比べ、違えば「先に変えられた」（`command.ErrRevisionConflict` → `RevisionConflictError`）を返す。試し直さない（後の変更が黙って勝つため）。保存のときの版の衝突（`domain.ErrVersionConflict`）も同じエラーにする。変わらない操作（同じ名前等）でも、revision が古ければ返す
  - 結果は対象の公開 ID と新しい revision だけにし、内容を返さない。対象を id で指さない操作（ゲーム全体で 1 つの集約の設定等）は id も返さない（どの操作も受け付けない識別子を見せないため。core#25）。画面は詳細を読み直し、詳細の revision を持つ（表示している内容と revision を同じ時点のものにそろえるため。投影が遅れているあいだは次の操作が「先に変えられた」になるが、ふだんの遅れは数ミリ秒）
  - 新しく作る操作（登録）と、画面を通らない操作（ログイン時の紐づけ・方針）には要らない
- 一覧の各要素からほかの read model の値を引くとき（プリセットデッキのカード名等）は、値を書き写さず、データローダー（presentation/graphql の dataloader.go）で引く（N+1 にしないため）。束ねて読む関数は、その read model の問い合わせ（query）と repository に置く。並べ方の決まりが要るなら query に置き（`query.CompareCards`）、presentation 層・studio に書き写さない
