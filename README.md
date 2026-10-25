# core

カルドセプト風ブラウザゲームのバックエンド（Go API）。

## 必要なもの

- Docker Engine + Docker Compose v2 以降
- make

想定環境は Linux / WSL2（ホストの uid 1000）。開発用コンテナは uid 1000 のユーザーで動き、生成物をホストユーザー所有にする。

Go・PostgreSQL はコンテナ内で動くため、ホストへのインストールは不要。

## セットアップ

```bash
make up      # PostgreSQL を起動し、migrations を適用してから app（air でホットリロード）を起動
make migrate # 未適用の migrations だけを Atlas で適用（DB を作り直さない）
make migrate-hash # migrations を足したら atlas.sum を作り直す
make gen-orm # migrations を足したら sqlboiler のモデル（internal/platform/orm/sqlboiler/models）を作り直す
make gen-api # GraphQL のスキーマを変えたら gqlgen のコードを作り直す
make seed-master    # 開発で触る見本のマスタ（カード・プリセットデッキ・スターターデッキセット）を studio のコマンドで登録（設定済みなら何もしない）
make publish-master # studio の cards（公開中・使用不可だけ）・starter_deck_sets・game_rules を core_game に写す（game が配るマスタを読めるようにする）
make fmt     # gofmt でフォーマットを修正
make lint    # gofmt・go vet・staticcheck・カスタムリンター（tools/linter）・migrations の検査
make test    # テンプレートの DB（core_template）を用意してから、コンテナ内で go test ./... -race を実行
make down    # 停止して DB のデータも破棄
```

| URL | 内容 |
|---|---|
| http://localhost:3333/version | バージョン情報 |
| http://localhost:3333/health | ヘルスチェック |
| http://localhost:3333/studio/graphql | studio 用の GraphQL（POST。studio の JWT が必須） |
| http://localhost:3333/game/graphql | game 用の GraphQL（POST。game の JWT が必須） |

PostgreSQL は `localhost:5432`（user: core_user / password: core_password / db: studio 用は core_studio、game 用は core_game）で接続できる。

開発で game のマスタ（配るスターターデッキ）を用意するときは、`make up` の後に `make seed-master`、続けて `make publish-master` を流す。seed-master は studio の DB に見本を登録し、投影が read model を書くまで待つ。publish-master はその read model（cards は公開中・使用不可のカードだけ。starter_deck_sets・game_rules）を core_game に写す（1 つのトランザクション。何度流しても同じ結果になる）。

ポートが他と衝突する場合は `CORE_PORT=13333 CORE_DB_PORT=15432 make up` のように変更できる。

## 他プロジェクトとの統合

`make up` すると共有ネットワーク `diecardomain` に参加する（無ければ作成する）。
game / studio は同じネットワーク経由で `http://core:3333` に接続する。core-db は参加しない。

## studio 用の GraphQL（/studio/graphql）

スキーマは `internal/service/studio/presentation/graphql/*.graphqls`（studio はこのファイルを参照する）。

- **認証**: すべての呼び出しに、studio（better-auth の jwt プラグイン）の JWT を `Authorization: Bearer <JWT>` で付ける。無い・不正なら 401（`extensions.code: UNAUTHENTICATED`。理由は返さない）。ネットワークの位置（内部からの呼び出しか）は信じない（game からも core に直接届くため）
- **検証**: 署名（EdDSA に固定。none・HMAC への差し替えを受け付けない）・`iss`・`aud`・期限。JWKS は studio-envoy を通して取る
- **JWKS の取り直し**: 1 時間ごとと、知らない kid の JWT が来たとき（5 分に 1 回まで）。起動時に studio が動いていなくても core は起動する（最初の JWT が来たときに取り直す。そのときも studio が動いていなければ、次は 5 分後）
- **studio の secret が漏れたとき**: studio の手順で jwks を作り直したら、core を再起動して古い鍵をすぐに捨てる（再起動しなくても 1 時間以内の取り直しで消える）
- POST だけを受け付ける（ミューテーションを GET で実行させる CSRF を防ぐため）
- **オペレーターの特定と紐づけ**: JWT の (iss, sub) に紐づいたオペレーターを、認可で使えるように context に載せる。まだ紐づいていなければ、JWT の確認済みのメールアドレス（`email`・`email_verified`。studio が JWT に入れる）で、登録済み・未紐づけのオペレーターを探して紐づける（初めてのログイン）。紐づけた後は (iss, sub) だけで識別し、メールアドレスは使わない。確認していないメールアドレスでは紐づけない（他人のメールアドレスでサインアップしても、オペレーターになれないように）
- JWT が正しければ、オペレーターに紐づかなくても入口では拒否しない。オペレーターかどうか・権限は、操作ごとに認可で判定し、各操作の結果の型（`ForbiddenError`）で返す

| 環境変数 | 内容 |
|---|---|
| `STUDIO_DB_DSN` / `GAME_DB_DSN` | studio 用・game 用の DB の接続先（開発・テストは同じ core-db の core_studio・core_game。別のサーバーにしてもよい） |
| `STUDIO_JWKS_URL` | studio の JWKS（開発・テストは `http://studio-envoy:10000/api/auth/jwks`） |
| `STUDIO_JWT_ISSUER` / `STUDIO_JWT_AUDIENCE` | studio の `JWT_ISSUER` / `JWT_AUDIENCE` と一致させる（`diecardomain-studio` / `diecardomain-core`） |
| `GAME_JWKS_URL` | game の JWKS（開発・テストは `http://game-envoy:10000/api/auth/jwks`） |
| `GAME_JWT_ISSUER` / `GAME_JWT_AUDIENCE` | game の `JWT_ISSUER` / `JWT_AUDIENCE` と一致させる（`diecardomain-game` / `diecardomain-core`） |
| `STUDIO_INITIAL_OPERATOR_EMAIL` | 初期オペレーターのメールアドレス（開発・テストは `operator@example.com`）。オペレーターが 1 人もいないときだけ、起動時に登録する。本人がこのアドレスで studio にサインアップし、確認してログインすると紐づく。2 回目以降の起動では、変えても何もしない |

どれも必須で、未設定なら起動しない（既定値で動くと、別の発行元の JWT を受け付ける等の設定の誤りに気づけないため）。開発用のコンテナには、テストが使う `TEST_DB_DSN`（core_test）も渡している（core は使わない。テストが開発の core の DB に触らないよう分ける）。

| 操作 | 内容 |
|---|---|
| `registerCreature` | Creature を名前で登録し、登録後の状態（公開用の ID・名前）を返す。結果は `RegisterCreatureSuccess` / `ValidationErrors`（名前が空・50 文字を超える）/ `CardNameAlreadyInUseError` / `ForbiddenError` |
| `registerItem` | Item を名前で登録し、登録後の状態（公開用の ID・名前）を返す。結果は `RegisterItemSuccess` / `ValidationErrors`（名前が空・50 文字を超える）/ `CardNameAlreadyInUseError` / `ForbiddenError` |
| `registerSpell` | Spell を名前で登録し、登録後の状態（公開用の ID・名前）を返す。結果は `RegisterSpellSuccess` / `ValidationErrors`（名前が空・50 文字を超える）/ `CardNameAlreadyInUseError` / `ForbiddenError` |
| `publishCard` | カード（Creature・Item・Spell のどれか。公開用の ID で指す）を公開中にする。準備中・使用不可から移れ、公開中にもう一度しても変わらない（revision も進まない）。結果は `PublishCardSuccess`（id・新しい revision）/ `CardNotFoundError` / `RevisionConflictError` / `InvalidCardStatusTransitionError` / `ForbiddenError` |
| `retireCard` | カードを使用不可にする。公開中から移れ、使用不可にもう一度しても変わらない。準備中からはできない。結果は `RetireCardSuccess` / `CardNotFoundError` / `RevisionConflictError` / `InvalidCardStatusTransitionError` / `ForbiddenError` |

カードは状態（`CardStatus`: 準備中 `DRAFT`・公開中 `PUBLISHED`・使用不可 `RETIRED`）を持ち、登録したときは準備中。準備中は自由に変えられ、プリセットデッキに入れられず、`make publish-master` で game に写さない。公開中はプリセットデッキに入れられ、game に写す。使用不可は新しくプリセットデッキに入れられないが、プレイヤーが持っているので game には写す。`cards` の一覧は `filter.statuses` で状態を絞り込め、`Card.status`・`Card.revision` を返す。プリセットデッキに新しく入れる・枚数を増やすカードが公開中でなければ、`setPresetDeckCardCounts` は `FieldUnavailable`（入力の誤り）を返す。すでに入っているカードは、使用不可になっても残せる。

スキーマは共通部分（`common.graphqls`）と機能ごとのファイル（Query・Mutation を `extend` する）に分ける。操作の結果は `<操作>Result = 成功の型 | エラー型` の union で返し（成功の型は、ミューテーションは `<操作>Success` で包み、クエリは問い合わせたもの。1 件なら対象の型、一覧なら 1 ページの `XxxConnection`）、ドメインエラーは GraphQL の `errors` ではなく型で表す（呼び出し側が分岐を型で漏れなく書けるため。設計メモ）。未認証は操作の結果ではないため union に含めず、HTTP 401 で返す。

- **層の分担**: presentation 層（GraphQL）は application 層の型だけを使い、ドメインの型（`operator.ID`・`card.Name`・集約の状態・ドメインのエラー）を知らない。application 層が窓口になり、素の値を受け取ってドメインの型に変え、application 層の結果の型とエラーを返す（ドメインを変えても presentation 層に及ばないように。参照の api は application 層が GraphQL の型を受け渡していたが、そうしない）
- **認可**: オペレーターを要する操作は、resolver の最初に紐づいたオペレーターかを確かめ（`requireOperator`。application 層の `authentication.HasOperator`）、違えば `ForbiddenError` を返す（core#10 でパーミッションの判定に広げる）。操作した人は、application 層のコマンドが context から取り出す
- **入力の誤り**: application 層が `input.Errors`（internal/service/studio/application/input。理由ごとの型 `input.Required`・`input.TooLong{Max}`・`input.OutOfRange{Min, Max}`・`input.InvalidFormat` の一覧）を返し、resolver が `ValidationErrors`（`FieldError` のインターフェースと、理由ごとの型 `FieldRequired`・`FieldTooLong{max}`・`FieldOutOfRange{min, max}`・`FieldInvalidFormat`）に写す。理由によって null になる項目は作らない。文言は返さず、studio が `__typename` と値から作る（上限などの値も core から受け取り、studio に書き写さない）
- **DB のエラーのログ**: 重複（一意性の表）や版の衝突（イベントストア）を、一意制約違反のエラーで受けない。PostgreSQL がふだんの重複のたびに ERROR のログを出し（重複は異常ではないのにログが埋まる）、トランザクションも中断されるため。一意性の表は参照の api と同じく `INSERT ... SELECT ... WHERE NOT EXISTS (ほかの ID が同じ値) ON CONFLICT (id) DO UPDATE ... RETURNING` で、登録にも変更にも使い、行が返らなければ重複とする（まれな同時の競合による一意制約違反も重複として返す）。イベントストアは `ON CONFLICT DO NOTHING RETURNING` で、行が返らなければ版の衝突とする
- **カード名**: Creature・Item・Spell で共有し、前後の空白を除いて NFKC で正規化した値で、3 種類をまたいで重複させない（一意な key の種別 `card.name`。表示は入力どおり）。上限は 50 文字（正規化する前の値で数える）
- **一意性の制約**: 制約ごとの専用の表ではなく、1 つの表 domain_unique_keys（key の種別の中で key が一意）で持つ。何を一意にするか（key の種別と key の作り方）はドメインが宣言し、インフラは確保の仕組みだけを持つ（制約を足すたびに表・SQL を足さないため。1 つの表で全集約のイベントを持つ domain_events と同じ考え方）。210 万件での計測では、専用の表と比べて引く・確保する速さの差は誤差の範囲で、容量は key の種別を数の ID にして抑えた（core#11）。中身は確認用の VIEW `domain_unique_key_view` で読める（列は unique_key_type_id・key・entity_id）
- **数の上限の制約**: 一意性・参照と並ぶ 3 つ目の制約の表 domain_quotas（上限の種別 × 持ち主ごとに使っている数）と domain_quota_types（core#39）。デッキは 1 人にゲーム全体のルール（`game_rules`）の `player.max_decks`（既定は 10 個）まで（種別 `deck.per_player`）。確保は作るコマンドと同じトランザクションの 1 つの文（`INSERT ... ON CONFLICT DO UPDATE SET used = used + 1 WHERE used < 上限 RETURNING`）で、上限なら行が返らず `ErrLimitReached` とする（エラーにせず、トランザクションも中断しない）。同時に取り合っても DB が同じ行を順番に更新するので上限を超えず、流し直しは要らない。上限の値は表にも CHECK にも持たず文に渡す（下限の 0 以上だけ CHECK）。使っている数はイベントから作り直せない、書く側の整合性のための記録。上限の値は game の `application/rules` が、写した `game_rules` から読んで返す（まだ写されていなければ、既定の値には戻さずエラー。調整は core#23）。デッキの名前は持ち主ごとに一意（`deck.name`）で、比べ方は `domain.NormalizeName`（カード名・プリセットデッキと同じ）。中身は確認用の VIEW `domain_quota_view` で読める

## game 用の GraphQL（/game/graphql）

スキーマは `internal/service/game/presentation/graphql/*.graphqls`（game は `app schema game` の出力を参照する）。操作は `registerPlayer`・`me`（core の版は、GraphQL ではなく HTTP の `/version` で返す）。

- **registerPlayer**（ミューテーション）: JWT の (iss, sub) のプレイヤーを登録し、公開 ID（`player { id }`）を返す。何度呼んでも、プレイヤーは (iss, sub) ごとに 1 人だけで、登録済みなら何も作らず同じ形（同じ id）で返す。同時に呼ばれても 1 人だけ
- **me**（クエリ）: 自分のプレイヤーを返す。`starterGrant` は read model `players` から読み、行がまだ無い（投影の遅れ）・配布が始まっていないときも `IN_PROGRESS`（画面にとっては「まだ届いていない」と同じ）。`registerPlayer` の結果の `player` からも取れる。未登録なら `PlayerNotRegisteredError`。game は、これが返ったときに `registerPlayer` を呼び、`me` を呼び直す

- **認証**: studio 用と同じく、すべての呼び出しに game（better-auth の jwt プラグイン）の JWT を付ける。検証（署名・`iss`・`aud`・期限）、JWKS の取り直し、401 の返し方、POST だけを受け付けることも studio 用と同じ。JWKS は game-envoy を通して取る
- **発行元の区別**: `aud` は studio 用と同じ `diecardomain-core` で、`iss`（`diecardomain-game`）で発行元を区別する。studio の JWT は game の入口では通らない（逆も同じ）
- **DB**: game の入口の操作は、game 用の DB（core_game）だけを読み書きする（core#33）
- **ゲーム全体のルール**: デッキの合計の枚数・同じカードの枚数・持てる数の上限など、運営が決める全体の値は、1 つの集約 `game_rules`（studio のサービス。一意な key `game_rules.singleton` でゲーム全体に 1 つ）が持つ（core#43）。値は値オブジェクト `gamerules.Rules`（internal/domain/gamerules。何の決まりかで `player`・`deck`（デッキに共通）・`starter_deck` に分け、JSON はスネークケース）で、studio と game が同じ型を使う。studio が起動時に既定の値で作る（あれば何もしない。複数台で先を越されても成功）。値の変更の操作はまだ無い（core#23）。studio は集約をイベントストアで読んで確かめる（投影の遅れで古い値を見ないため）。game は read model `game_rules` を `make publish-master` で写したものを読み、写されていなければ既定の値に黙って戻さずエラー（配布のポリシーでは、写されれば直る失敗）。カード名・デッキ名の長さは値の形の決まりとして `game_rules` に入れない
- **スターターの配布**: プレイヤーを登録すると、それをきっかけにポリシー（`starter_grant`）が、公開済みのマスタ（`make publish-master`）から所持カードとデッキ 2 つを配る。マスタが未公開のあいだは、間隔を空けてやり直し、公開されたら配られる。所持カードはカードごとに 2 つのデッキのうち多いほうの枚数。配布の状態は `me` の `Player.starterGrant`（`IN_PROGRESS`・`COMPLETED`）で返す
- **プレイヤーの特定**: JWT が正しければ、プレイヤーに登録されていなくても拒否しない（初めての `registerPlayer` が登録するため）。誰のプレイヤーかは入力ではなく JWT の (iss, sub) だけで決める

## イベントストア

DDD + CQRS + Event Sourcing（設計メモ）。参照プロジェクトの api の形に、設計メモの 6 節の修正点を反映し、さらに次の点を変えた（#7）。

- 種別のレジストリ（`domaindef.Registry`）はパッケージの変数にせず、起動時に読み込んで `identifier`・`eventstore` に渡す
- 種別名・イベント名は、型が宣言する（ID の `EntityType()`、イベントの `EventType()`）。種別の id と名前はコードの一覧（`domaindef.Types`）が決め、起動時に DB ごとに `domaindef.Sync` で足してから、`Registry.Verify...` で照合し、食い違えば起動しない（core#33）
- イベントのスキーマの版は、イベントの型が宣言する（`SchemaVersion()`）。読み込み時は保存された版を `Unmarshal` に渡す
- 集約を足したら: 種別の一覧（`domaindef.Types`）にその集約とイベントを足し、起動時の `Verify` と `domaindef` のテストにその集約の `Types()` を足す（表が要るときは、マイグレーションも足して `make migrate-hash`・`make migrate`・`make gen-orm`）

中身を確かめるときは、種別の id を名前に展開した確認用の VIEW（`domain_entity_view`・`domain_event_view`・`domain_event_type_view`）を使う（アプリからは使わない）。

テストは `TEST_DB_DSN` のサーバーだけを使い、開発の core の DB（core_studio・core_game）には触らない（core#33）。DB を使うときは、パッケージごとにどちらか一方にそろえる（パッケージを見れば、共有の core_test に触るかがわかるように）。

- `rdbtest.Tx`: 共有の core_test で、テストの終わりに rollback する。1 つのトランザクションで済むパッケージ（domaindef・constraint・eventstore 等）
- `rdbtest.NewDatabase`: テンプレートの DB（`core_template`）から自分だけの DB を複製する（テストの終わりに消える）。commit するパッケージ（同時実行・HTTP のリクエスト・イベント駆動）と、ほかのデータが無いことを前提にするパッケージ（studio/app/command・studio/infrastructure/repository・eventdriven・platform/rdb）

共有の core_test に commit すると行が残り、ほかのテストの前提を崩すため、commit する道具（`Open`）は外に出さない。

結合テスト（tests/integration）は、`TestMain` で `rdbtest.CloneDatabase` により studio 用・game 用の DB を複製し、`internal/wiring` で組み立てたアプリ（Hub を含む）と共有して並列に流す。core_test とテンプレートは `make test`（`migrate-template`）と CI が、`tools/createdb` で作ってマイグレーションと種別（`tools/synctypes`）を流しておく。テストの種類と DB の使い分けの議論は core#9。

## テスト用環境（compose.test.yaml）

game / studio のテスト用に、使い捨ての core と DB を起動する compose。利用側の compose から include して使う。

- core は本番用イメージ（`.build/Dockerfile`、distroless）で動かす。air は使わない
- ホストにポートを公開せず、共有ネットワークにも参加しない。開発用の core と同時に起動できる
- DB のデータは tmpfs に置き、終了とともに破棄する

単体で試す場合:

```bash
docker compose -p core-test -f compose.test.yaml up -d --build --wait
docker compose -p core-test -f compose.test.yaml down -v
```

## リリースとイメージ配布（CI）

| パイプライン | ジョブ | 内容 |
|---|---|---|
| マイルストーンのブランチに向かう MR（Issue の MR） | issue-mr（手動。実行しない） | 何もしない。パイプラインは skipped になり、「スキップされたパイプラインを成功とみなす」の設定でマージできる |
| main に向かう MR（マイルストーンの MR） | lint / test | `make lint` / `make test` 相当 |
| main に向かう MR | build-mr（手動） | `registry.gitlab.com/aimerzarashi/diecardomain/core:mr-<MR番号>` と `.../core/migrate:mr-<MR番号>` に push |
| main | release | semantic-release がコミットからバージョンを決め、タグ（`v1.2.0`）と GitLab リリースを作る。リリース対象のコミットが無ければ何もしない |
| タグ（`v*`） | deploy | Docker Hub の `diecardomain/core:<タグ>`・`:latest` と `diecardomain/core-migrate:<タグ>`・`:latest` に push |

バージョンは Conventional Commits で決まる（`feat:` → マイナー、`fix:` / `perf:` → パッチ、`BREAKING CHANGE` → メジャー）。

マイルストーンのブランチに向かう Issue の MR では、何もしない手動のジョブだけを置いて実行しない（lint・test は手元の `make lint`・`make test` で確かめる。パイプラインを作らないと「パイプラインが成功する必要がある」の設定でマージできず、その設定は main に向かう MR の歯止めとして残すため）。main への push では lint・test を流さない（直前の main に向かう MR で同じ中身を確かめているため）。main は直接 push できない設定（保護ブランチの push は No one）にして、すべての変更が MR の lint・test を通るようにしている。共有 Runner の時間を減らすため（core#30）。

## カスタムリンター（tools/linter）

参照プロジェクトの api のカスタムリンターを移植したもの（#8）。`make lint` と CI の lint で `go run ./tools/linter ./...` を実行する。対象の設定は `tools/linter/main.go` に書く（`make lint` と CI で同じ設定にするため）。

| 検査 | 内容 |
|---|---|
| factory_enforcer | `internal` の値の型（`type Xxx string` 等）を、ほかのパッケージで型変換・生の値の代入で作ることを禁止し、生成関数（`NewXxx`）を使わせる。internal 全体を対象にするのは、値の型を足したパッケージを対象に入れ忘れないため。テストも対象（検証を通らない値でテストしないため）。生成コード（先頭に `Code generated ... DO NOT EDIT.` のあるファイル。sqlboiler・gqlgen 等）は自動で除く（生成関数を持たず、手で直せないため） |
| import_alias_forbidder | core の中のパッケージを別名で import することを禁止する（外部のライブラリは名前の衝突で別名が要るため対象外）。生成コードは調べない |
| testify_msgandargs_forbidder | testify の検査にメッセージを付けることを禁止する（テストの名前で仕様がわかるようにするため） |
| test_context_enforcer | テストで `context.Background()`・`context.TODO()` を使わず `t.Context()` を使わせる |
| trun_naming_enforcer | サブテスト（`t.Run`）の名前を、英小文字のスネークケース（`[a-z0-9]` を `_` でつなぐ）にし、兄弟（同じテーブル・同じ親）で重複させない。文字列リテラルの名前だけを調べる（入力から作る名前は調べない） |

サブテストの名前は、Google の Go スタイルガイドに沿って、文章ではなく識別子のように付ける（例: `blank_subject`、`rejects_blank_issuer`）。その行の条件（と期待する結果）を表し、詳しい説明は失敗時のメッセージに書く。
`go test -run` で絞り込みやすく、テストの実行器が空白を `_` に置き換えて読みにくくならず、`/` が絞り込みの区切りと衝突しないため。
サブテストは、同じ振る舞いを入力を変えて確かめるテーブル駆動のときだけ使い、1 つの振る舞いは 1 つのテスト関数（`Test<対象>_<振る舞い>`）に書く。

参照の api の「正常系/」「異常系/」で始める規則は、`/` を含むうえ、分類は付いても何を確かめるかが名前に出ないため採らなかった。
名前に中身があるか（`case1` のような名前でないか）はレビューで見る。中身の無い名前の一覧をリンターに持つと、一覧を育て続けることになるため。

## DB マイグレーション

[Atlas Community Edition](https://atlasgo.io/community-edition)（`arigaio/atlas:1.3.3-community`）で、`migrations/NNNN_name.sql` のうち未適用のものだけを番号順に適用する。
core・studio・game で同じ形式・同じ実行役にそろえている（方針は #5）。

| ファイル | 内容 |
|---|---|
| `migrations/NNNN_name.sql` | PostgreSQL の SQL（手で書く） |
| `migrations/atlas.sum` | Atlas が作るハッシュの一覧。SQL と一緒にコミットする |
| `migrations/BASELINE` | 並びをどの作り直しから始めたかの印（1 行目）。`scripts/check-migrations.sh` が読む |
| `local/migrate/Dockerfile`・`atlas.hcl` | マイグレーション用イメージ（Atlas + migrations）。DB の接続先は実行時に `DATABASE_URL` で渡す |

### 追加の手順

1. `migrations/` に、その時点の最大の番号 + 1 で SQL を置く
2. `make migrate-hash` で `atlas.sum` を作り直す
3. `make migrate` で開発用の DB に適用する
4. SQL と `atlas.sum` を一緒にコミットする

### 決まり

- ファイル名は `NNNN_name.sql`（番号は 4 桁の数字、名前は小文字・数字・`_`）。Atlas は最初の `_` までを番号として読む
- 表の作り（DDL）だけを書き、studio 用・game 用の両方の DB に同じものを流す。種別・購読者の行は入れない（コードの一覧から、core が起動時に入れる。core#33）
- 追加のみ。down は無い。誤りは新しいマイグレーションで直す。適用済みのファイルは変更・削除・改名しない
- 例外として、リリース前（本番の DB ができる前）に限り、並びを作り直してよい。作り直したら `migrations/BASELINE` の 1 行目を書き換える（`check-migrations.sh` は比較元とこの行が違えば比べ合わせを飛ばす）
- マージで `atlas.sum` が衝突したら、自分のファイルの番号を付け直して `make migrate-hash` し直す
- トランザクション内で実行できない文（`CREATE INDEX CONCURRENTLY` 等）は、ファイルの先頭に `-- atlas:txmode none` を書き、**直後に空行を入れる**（空行が無いと指示が無視され、トランザクション内で実行されて失敗する）

  ```sql
  -- atlas:txmode none

  CREATE INDEX CONCURRENTLY cards_name_idx ON cards (name);
  ```

### Atlas が保証すること

- 何度実行しても、未適用のものだけを適用する。複数で同時に実行しても 1 回だけ適用される（ロック）
- 1 ファイルを 1 トランザクションで適用する。途中で失敗したらそのファイルの変更は残らず、未適用のまま止まる
- 番号の抜け（適用済みより小さい番号を後から足す）はエラーで止める
- `atlas.sum` と食い違うファイル（`make migrate-hash` のし忘れ）は「checksum mismatch」で止める
- 記録は `atlas_schema_revisions.atlas_schema_revisions` テーブル。実行時間はジョブのログ（CLI の出力）で確認する（`execution_time` 列の値は CLI の表示と合わず、意味を確認中）

### Atlas が検出しないこと（`scripts/check-migrations.sh` で補う）

Atlas は以下を黙って通す（`migrate validate` / `migrate lint` でも検出しない）ため、`make lint` と CI の lint ジョブで検査する。
適用済みファイルの変更は、比較元からの差分で見る（`make lint` は origin/main との分岐点、CI は main に向かう MR の対象ブランチとの分岐点）。main は直接 push できない設定のため、MR を経ない変更は無い。lint が通らなければ build-mr も動かない。

| 検査 | Atlas に任せた場合 |
|---|---|
| 番号の重複 | 後のファイルが適用されないまま status が OK になる。未適用同士の重複は片方だけ適用して panic する |
| 適用済みファイルの変更・削除・改名（分岐点からの差分） | `atlas.sum` を作り直されると apply は何もせず通り、DB とファイルが食い違う |
| 比較元の最大より小さい番号の追加 | 空の DB に全部を適用するテストは通り、本番（適用済みの DB）で初めて out of order で止まる |
| ファイル名の形式 | `0002_5_x.sql` は番号 `0002` と読まれ、重複になる |
| 指示行の直後の空行 | `-- atlas:txmode none` が無視される |

| 環境 | 実行方法 |
|---|---|
| 開発（compose.yaml） | core-migrate-studio・core-migrate-game サービス（migrations をマウント）が core の起動前に 1 回実行。`make migrate`（種別の行も入れる） |
| テスト用（compose.test.yaml） | core-migrate-studio・core-migrate-game サービス（マイグレーション用イメージ） |
| CI | test ジョブで Atlas（v1.3.3）を取得し、適用してから go test |
| リリース・本番 | タグのパイプラインで `diecardomain/core-migrate:<タグ>` を Docker Hub に push。本番では core より先に、studio 用・game 用の DB それぞれの `DATABASE_URL` を渡してジョブとして実行する |
