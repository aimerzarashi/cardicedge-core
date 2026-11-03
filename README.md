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
make seed-master    # 開発で触る見本のマスタ（カード・プリセットデッキ・配布デッキ・ボード・NPC のアバター・NPC のデッキ）を studio のコマンドで登録（配布デッキはプリセットデッキから写して作って公開する。ボードはレイアウトを入れて公開する。NPC のデッキはプリセットデッキから同じ名前で写して公開する。NPC のアバターは登録して公開中のデッキを割り当て、公開する。作成済みなら飛ばす）
make publish-master # studio の cards（公開中・使用不可だけ）・grant_decks（全行を状態つきで）・boards・npc_avatars（cards と同じく公開中・使用不可だけ）・npc_decks（写した npc_avatars に割り当てられたものだけ。状態つき）・game_rules を core_game に写す（game が配るマスタを読めるようにする）
make fmt     # gofmt でフォーマットを修正
make lint    # gofmt・go vet・staticcheck・カスタムリンター（tools/linter）・migrations の検査
make test    # テンプレートの DB（core_template）を用意してから、コンテナ内で go test ./... -race を実行
make down    # 停止して DB のデータも破棄
make clean   # down に加えて、トレースとログ（otel-lgtm のデータ）と core-db のログも破棄
```

| URL | 内容 |
|---|---|
| http://localhost:3333/version | バージョン情報 |
| http://localhost:3333/health | ヘルスチェック |
| http://localhost:3333/studio/graphql | studio 用の GraphQL（POST。studio の JWT が必須） |
| http://localhost:3333/game/graphql | game 用の GraphQL（POST。game の JWT が必須） |

PostgreSQL は `localhost:5432`（user: core_user / password: core_password / db: studio 用は core_studio、game 用は core_game）で接続できる。

開発で game のマスタ（新しいプレイヤーに配る配布デッキ）を用意するときは、`make up` の後に `make seed-master`、続けて `make publish-master` を流す。seed-master は studio の DB に見本を登録し、投影が read model を書くまで待つ。publish-master はその read model（cards は公開中・使用不可のカードだけ。grant_decks は全行で、game は公開中だけを配る。boards は cards と同じ選び方で、game は公開中だけを見せる。game_rules）を core_game に写す（1 つのトランザクション。何度流しても同じ結果になる）。

ポートが他と衝突する場合は `CORE_PORT=13333 CORE_DB_PORT=15432 make up` のように変更できる。

## トレースとログの受け手（grafana/otel-lgtm）

`make up` で、トレース（Tempo）・ログ（Loki）の受け手と Grafana が入った 1 コンテナ（`otel-lgtm`。版は compose.yaml で固定）も起動する。データ（トレースとログ）は名前つきのボリューム（`diecardomain-otel-lgtm-data`）に置き、PC の再起動・コンテナの作り直し・`make down` では消えない（前の日の調べたいトレースとログを残すため。core#82）。消すときは `make clean`。core は core#75 で送るようにした。studio・game が送る設定は studio#50・game#25 で入れる。

| 用途 | 場所（既定のポート） | 変える環境変数 |
|---|---|---|
| Grafana の画面 | http://localhost:3002（127.0.0.1 にだけ公開。ログインなしで管理者として入れる） | `OTEL_GRAFANA_PORT` |
| OTLP gRPC | `host.docker.internal:4317` | `OTEL_GRPC_PORT` |
| OTLP HTTP | `host.docker.internal:4318`（ホストからは `localhost:4318`） | `OTEL_HTTP_PORT` |

- 届け方: 受け手は共有ネットワークに入れない（game から studio に内部から届く経路を作らないため。core#45）。OTLP の口をホストに公開し、送る側のコンテナは `extra_hosts: ["host.docker.internal:host-gateway"]` を付けて `host.docker.internal` で送る。開発の環境でもテスト用の環境（studio・game の compose.test.yaml）でも同じ形で届く
- OTLP の口を `127.0.0.1` にだけ公開する案は採らなかった。コンテナから見たホストは docker のブリッジのゲートウェイで、ループバックだけで待つ口には届かないため

## 重い SQL・遅い SQL と実行計画（core#83）

開発の core-db は PostgreSQL 自身に SQL の集計とログを取らせ、Grafana（上の otel-lgtm）で見る。アプリのコードは関係しない。開発の環境だけで、compose.test.yaml・リリース確認の環境には入れない。

| 部品 | 役目 |
|---|---|
| core-db（compose.yaml の `command`・local/core-db/extensions.sql） | pg_stat_statements（SQL の形ごとの回数・時間の集計。core_studio・core_game それぞれ）、100ms 以上の SQL のログ（log_min_duration_statement）、その実行計画（auto_explain。JSON）を有効にする。ログは JSON の行で名前つきのボリューム（`diecardomain-core-db-log`）に書く |
| otel-lgtm の中の Collector（足す設定は local/otel-lgtm/otelcol-core-db.yaml。compose.yaml の OTELCOL_EXTRA_ARGS で渡す） | ボリュームを読み取り専用で受け、JSON の行を読んで otel-lgtm の Loki に送る（`service_name="core-db"`。時間は `duration_ms`、種類は `kind`（`statement`・`plan` 等）、SQL は `sql`、実行計画は `plan`）。読んだ位置は otel-lgtm の /data に残し、作り直しても重ねて送らない |
| otel-lgtm（local/grafana） | provisioning のディレクトリにファイルで足したデータソース（`core-db core_studio`・`core-db core_game`）とダッシュボード「core-db の SQL」 |

見方: http://localhost:3002 のダッシュボード「core-db の SQL」（上の変数 DB で core_studio・core_game を選ぶ）。

- 重い SQL: pg_stat_statements の表（合計時間・呼ばれた回数・平均・最大・行数。列の見出しで並べ替える）。起動してからの累計で、リセットは `SELECT pg_stat_statements_reset()`
- 遅い SQL: Loki の `{service_name="core-db"} | duration_ms != "" | kind != "plan"`（時刻・時間・SQL）
- 実行計画: Loki の `{service_name="core-db"} | kind = "plan"`（auto_explain。計画の JSON に `Query Text` が入る）
- 閾値は compose.yaml の core-db の `log_min_duration_statement`・`auto_explain.log_min_duration`（100ms）。調べるときに下げる（反映は core-db の作り直し）
- 起動の設定は、次の `make up` で core-db のコンテナが作り直されて効く（docker compose は名前の無いボリュームを引き継ぐので、データは消えない）。拡張を作る初期化の SQL（local/core-db/extensions.sql）は新しい DB でしか流れないので、すでにある DB には `docker exec core-core-db-1 psql -U core_user -d core_studio -f /docker-entrypoint-initdb.d/extensions.sql` で手で流す（`make down` で DB を作り直したときは自動で流れる。そのときは `make seed-master`・`make publish-master` で見本のマスタを入れ直す）。ログのボリュームは `make down` では残り、`make clean` で消える
- PostgreSQL のログは標準エラーに出なくなるので、`docker logs core-core-db-1` には出ない（Grafana か、ボリュームのファイルで見る）

## 処理の遅れ・誤り・GraphQL の時間（core#84）

http://localhost:3002 のダッシュボード「core の処理（遅れ・誤り・GraphQL の時間）」。ダッシュボードは local/grafana/dashboards に JSON を置けば、otel-lgtm が読み込む（provisioning。local/grafana/provisioning/dashboards/diecardomain.yaml）。タグに `diecardomain` を付け（Grafana の一覧で、otel-lgtm に最初から入っている見本のダッシュボードと分けて絞り込むため）、中身を表すタグ（`core`・`core-db` 等）を添える。

| 欄 | 元のデータ | 見ること |
|---|---|---|
| 投影・ポリシーの遅れ | core の処理のログの `lag_ms`（Loki）の 95 パーセンタイル | 遅れの大きい投影・ポリシー。テストが read model を待ちきれない原因になりうる |
| 投影・ポリシーの処理の時間 | 同じログの `duration_ms` | 処理そのものが遅いか（遅れが大きく処理の時間が短ければ、待ちが原因） |
| 投影・ポリシーの失敗の数 | 同じログの `result="failure"` | 失敗の理由は、同じ時刻の ERROR・WARN のログで見る |
| ERROR・WARN のログの数 | core・studio・game のログ（Loki）の `detected_level` | 中身は Explore で `{service_name="core"} \| detected_level="error"` のように引く |
| GraphQL の操作ごとの時間・呼び出しの数 | トレースから otel-lgtm が作る指標（Prometheus の `traces_spanmetrics_*`） | 遅い操作は、Tempo で個々のトレースを見る |

## core のトレースとログ（OpenTelemetry。core#75）

core は、HTTP の呼び出し・GraphQL の操作とリゾルバー・投影とポリシーの処理のスパンと、ログを、上の受け手へ送る。初期化は cmd/app の `run`（`internal/platform/telemetry` の `Setup`）、終了は `serve` のあとに送り切る（`shutdownTimeout` の中の `telemetryFlushTimeout`）。

| 環境変数 | 意味 |
|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | 送り先（開発・テストの compose は `http://host.docker.internal:4318`）。**無ければ送り出しを作らず、標準出力のログだけ**（何も送らず、何も待たない。結合テストは設定しないのでこの形で動く） |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` を設定する。送り出しは HTTP だけで、この値で gRPC には切り替わらない |
| `OTEL_SERVICE_NAME` | サービス名（core は `core`）。`OTEL_RESOURCE_ATTRIBUTES`・信号ごとの送り先（`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` 等）も SDK が読む |

- これらは platform/config の「環境変数はすべて必須」の例外（任意。config に読み込まず SDK が読む）。受け手が止まっていても core は普通に動き、届かない内部の誤りは WARN で 1 分に 1 回までしか出さない（ERROR にしない。結合テストは ERROR のログで全体を失敗にするため）
- 引き継ぎは W3C の TraceContext（`traceparent`）だけ。受けた `traceparent` の子としてスパンを作る。`/health` はスパンにしない
- スパン: HTTP（`internal/platform/telemetry` の `HTTP`。`wiring.Start` が返す入口の外側）、GraphQL の操作とリゾルバー（`internal/platform/gqltrace`。studio・game の handler が `srv.Use` する。単純なフィールドは作らない）、投影とポリシーの処理（`internal/eventdriven`。下の「追い方」。core#85）
- ログ: 標準出力の JSON に、スパンの中で出したログ（`slog.InfoContext(ctx, ...)` 等）へ `trace_id`・`span_id` を足し、同じログを OpenTelemetry のログとして Loki にも送る。`ctx` を渡さない `slog.Info` にはトレースの情報が付かない
- 追い方: Grafana の Explore で Loki から `{service_name="core"} |= "event saved"` のように探し、ログの `trace_id` で Tempo のトレースを開く。投影・ポリシーの処理のログ（`eventdriven: event processed`）は、イベントを受け取った別の流れだが、保存したときのトレースを `domain_events.traceparent`（W3C の traceparent。保存のときに ctx から書く。トレースが無ければ NULL。payload には入れない）に残しており、その `trace_id`（と保存のスパンの `span_id`）が付く。同じ `trace_id` で Loki から保存のログと処理のログが並ぶ。処理のログの `trace_id` は、投影のスパンのトレースではなく、保存したときのトレースのまま（投影のトレースの `trace_id` にすると、Loki で元の `trace_id` 1 つで引けなくなるため）。投影を作り直すと、昔のイベントの `trace_id` の処理のログが出る。トレースが無いイベント（初期データ等）の処理のログは `trace_id` を持たず、`event_id` で保存のログとつなぐ
- 処理のスパン（元のトレースがあるイベントの処理だけ。seed-master・起動時の作成など、トレースが無いイベントはスパンにしない）:
  - ポリシー: `policy <購読者の名前>`。イベントごとに、保存したときのトレースの子（元のトレースを開くと、保存のスパンの下に並ぶ）。属性は `subscriber_kind`・`subscriber_name`・`entity_type`・`entity_id`・`event_id`・`aggregate_version`・`event_type`（処理のログと同じ名前）。失敗はスパンの誤りとして残る
  - 投影: `projection <購読者の名前>`。いつも新しいトレース（件数で形を変えない）で、新しく反映したイベントの元のトレースすべてへリンクを張る。属性は `subscriber_kind`・`subscriber_name`・`entity_type`・`entity_id`・`aggregate_version`（作り直した後の版）・`event_ids`（新しく反映したイベントの `event_id` の一覧）
  - 探し方（Grafana の Explore で Tempo）: 元のトレース → 投影は TraceQL の `{ link:traceID = "<trace id>" }`。投影 → 元は、投影のスパンの詳細に出るリンクをたどる。名前・属性でも探せる（`{ name = "projection boards" }`、`{ span.event_id = 95 }`、`{ span.entity_id = 12 && span.subscriber_kind = "policy" }`。属性の名前は処理のログと同じなので、ログの値をそのまま使える）。送った直後の検索では、数十秒は出ないことがある

## 他プロジェクトとの統合

`make up` すると共有ネットワーク `diecardomain-game`（game 用）と `diecardomain-studio`（studio 用）の両方に参加する（無ければ作成する）。
game / studio はそれぞれのネットワーク経由で `http://core:3333` に接続する。ネットワークを分けるのは、game が乗っ取られても studio に内部から届く経路そのものを無くすため（core#45）。core-db は参加しない。

## studio 用の GraphQL（/studio/graphql）

スキーマは `internal/service/studio/presentation/graphql/*.graphqls`（studio はこのファイルを参照する）。

- **認証**: すべての呼び出しに、studio（better-auth の jwt プラグイン）の JWT を `Authorization: Bearer <JWT>` で付ける。無い・不正なら 401（`extensions.code: UNAUTHENTICATED`。理由は返さない）。ネットワークの位置（内部からの呼び出しか）は信じない（game からも core に直接届くため）
- **検証**: 署名（EdDSA に固定。none・HMAC への差し替えを受け付けない）・`iss`・`aud`・期限。JWKS は studio-envoy を通して取る
- **JWKS の取り直し**: 1 時間ごとと、知らない kid の JWT が来たとき（1 秒に 1 回まで。同時に来た JWT は順に待って、それぞれ取り直す。keyfunc の既定の 5 分に 1 回では、新しい鍵の JWT が同時に来ると 1 つ以外が 401 になるため。core#51）。起動時に studio が動いていなくても core は起動する（最初の JWT が来たときに取り直す）
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
| `grantDecks`（クエリ） | 配布デッキの一覧（名前の順。`filter.nameContains`・`filter.statuses` で絞り込み、`first`・`after` でページ分け）。結果は `GrantDeckConnection` / `ValidationErrors` / `ForbiddenError` |
| `grantDeck`（クエリ） | 配布デッキの詳細（公開用の ID で指す。名前・状態・カードと枚数・合計・写し元・revision 等）。結果は `GrantDeck` / `GrantDeckNotFoundError` / `ForbiddenError` |
| `registerGrantDeck` | 配布デッキを名前だけで登録する（空のカードで、準備中）。結果は `RegisterGrantDeckSuccess`（id・revision）/ `ValidationErrors` / `GrantDeckNameAlreadyInUseError` / `ForbiddenError` |
| `registerGrantDeckFromPresetDeck` | プリセットデッキ（公開用の ID で指す）からカードと枚数を写して、指定した名前で配布デッキを登録する（準備中）。結果は `RegisterGrantDeckFromPresetDeckSuccess`（id・revision）/ `ValidationErrors`（名前）/ `PresetDeckNotFoundError` / `GrantDeckNameAlreadyInUseError` / `ForbiddenError` |
| `renameGrantDeck` | 準備中の配布デッキの名前を変える。結果は `RenameGrantDeckSuccess`（id・新しい revision）/ `ValidationErrors` / `GrantDeckNotFoundError` / `GrantDeckNameAlreadyInUseError` / `GrantDeckNotDraftError` / `RevisionConflictError` / `ForbiddenError` |
| `setGrantDeckCardCounts` | 準備中の配布デッキのカードの枚数を変える（一覧のどれか 1 つでも誤りがあれば何も変えない）。結果は `SetGrantDeckCardCountsSuccess` / `ValidationErrors` / `GrantDeckNotFoundError` / `GrantDeckNotDraftError` / `RevisionConflictError` / `ForbiddenError` |
| `deleteGrantDeck` | 準備中の配布デッキを削除する（名前はほかの配布デッキが使えるようになる）。結果は `DeleteGrantDeckSuccess`（id）/ `GrantDeckNotFoundError` / `GrantDeckNotDraftError` / `RevisionConflictError` / `ForbiddenError` |
| `publishGrantDeck` | 配布デッキを公開中にする（準備中・使用不可から。すでに公開中なら変わらない）。合計がちょうど決まった枚数でない・入っているカードに公開中でないものがある場合は `ValidationErrors`（項目 `cards`）。結果は `PublishGrantDeckSuccess` / `ValidationErrors` / `GrantDeckNotFoundError` / `GrantDeckLimitReachedError`（公開中の数の上限。`max` を持つ）/ `RevisionConflictError` / `ForbiddenError` |
| `retireGrantDeck` | 配布デッキを使用不可にする（公開中から。すでに使用不可なら変わらない。新しいプレイヤーには配らない）。結果は `RetireGrantDeckSuccess` / `GrantDeckNotFoundError` / `InvalidGrantDeckStatusTransitionError` / `RevisionConflictError` / `ForbiddenError` |
| `gameRules`（クエリ） | ゲーム全体のルールのうち対戦の決まり（`match.minPlayers`・`match.maxPlayers`（参加人数の範囲。既定は 2〜4）・`match.maxHandCards`（手札の上限。既定は 6）。部屋の条件が読んで見せる）。read model `game_rules` を読み、まだ投影されていなければ既定の値に戻さず GraphQL の errors にする。ほかの項目（デッキなど）は、読む画面ができたときに足す。結果は `GameRules` / `ForbiddenError` |
| `studioRules`（クエリ） | studio でマスタを作るときの決まり（ボードの幅・高さの下限 `board.minSize`・上限 `board.maxSize`。画面が入力の時点で範囲の外を止めるために読む）。read model `studio_rules` を読み、まだ投影されていなければ既定の値に戻さず GraphQL の errors にする（待てば直る失敗）。結果は `StudioRules` / `ForbiddenError` |
| `boards`（クエリ） | ボードの一覧（名前の順。`filter.nameContains`・`filter.statuses` で絞り込み、`first`・`after` でページ分け。各要素がマスとつながり・対も持つ）。結果は `BoardsConnection` / `ValidationErrors` / `ForbiddenError` |
| `board`（クエリ） | ボードの詳細（公開用の ID で指す。名前・状態・形・幅と高さ・既定のルール `defaultRules`・マスの数 `tileCounts`（土地の合計・拠点・チェックポイント・属性ごとの土地の数・転送円・橋・聖堂・ほこら・占い館・カード屋の数。投影のときに数える）・公開できない理由 `publishProblems`（`BaseCountProblem`・`NoCheckpointProblem`・`UnreachableTilesProblem`・`UnpairedTilesProblem` を `__typename` で見分け、座標を持つ理由は `positions` を持つ。空なら公開できる。投影のときに計算するので、保存のたびに読んで分かる）・マス・つながり・対 `pairs`（転送円どうし・橋どうし。a → b の順）・revision 等）。結果は `Board` / `BoardNotFoundError` / `ForbiddenError` |
| `checkBoardLayout`（クエリ） | ボードのレイアウト（幅・高さ・マス・つながり・対の全体。`changeBoardLayout` の入力から id・revision を除いたもの）を、保存せずに判定する。ボードを指さず、何も保存せず、イベントも作らない（revision も使わない）。入力の誤りは `changeBoardLayout` と同じ決まり・同じ項目のパスの `ValidationErrors`（`newLayout` を共有し、決まりを 2 か所に書かない）。誤りが無ければ、公開するときと同じ判定（`CheckReadiness`）の公開できない理由 `publishProblems`（`BoardNotPublishableError.problems` と同じ型・順。空なら公開できる）。権限は保存と同じ（紐づいたオペレーターだけ）。結果は `CheckBoardLayoutSuccess` / `ValidationErrors` / `ForbiddenError`。画面が保存の前に、公開できるかを見せるために使う |
| `registerBoard` | ボードを名前だけで登録する（四角形で、大きさは 8×8 を `studio_rules` の範囲に収めた値。マスもつながりも無い準備中。既定のルールは登録の直後の値: 目標の魔力 8000・開始時の魔力 600・周回のボーナス 300・チェックポイントのボーナス 100・ラウンドの上限 30・サイコロの目 1〜6）。結果は `RegisterBoardSuccess`（id・revision）/ `ValidationErrors` / `BoardNameAlreadyInUseError` / `ForbiddenError` |
| `renameBoard` | 準備中のボードの名前を変える。結果は `RenameBoardSuccess` / `ValidationErrors` / `BoardNotFoundError` / `BoardNameAlreadyInUseError` / `BoardNotDraftError` / `RevisionConflictError` / `ForbiddenError` |
| `changeBoardLayout` | 準備中のボードのレイアウト（幅・高さ・マス・つながり・対の全体）を変える。前との差分は core が計算し、変わらなければ revision も進まない。幅・高さの範囲は `studio_rules`（`studioRules.board`）の値で、範囲の外は `FieldOutOfRange`（`min`・`max` も `studio_rules` の値）。マス（`tiles`）の各要素は `BoardTileInput`（`@oneOf`。`land { x y attribute }`・`base`・`checkpoint`・`warp`（転送円）・`bridge`（橋）・`sanctuary`（聖堂）・`shrine`（ほこら）・`prophecyShop`（占い館）・`cardShop`（カード屋）（土地以外は `{ x y }` だけ）のうちちょうど 1 つを書く。書いた項目が種類になり、土地の属性が無い・土地以外に属性がある組み合わせは入力の形として作れない。「ちょうど 1 つ」は変数で送られた値だと GraphQL の層が確かめないため、core が確かめて `tiles.N` の `FieldInvalidFormat` にする）。入力の誤りは `ValidationErrors`（項目 `width`・`height`・`tiles.N.land.x`・`tiles.N.land.y`（`land` は書いた種類の名前で、`base`・`warp`・`prophecyShop` 等もある）・`tiles.N`・`connections.N.a`・`connections.N.b`・`connections.N`・`pairs.N.a`・`pairs.N.b`・`pairs.N`。理由は `FieldOutOfRange`・`FieldDuplicated`・`FieldNotFound`・`FieldNotAdjacent`、対は端が転送円でも橋でもない `FieldNotPairable`・転送円と橋を組む `FieldPairKindMismatch`・自分自身と組む `FieldPairedWithSelf`・すでに別の対に入っているマスは `FieldDuplicated`（`pairs.N.a`・`pairs.N.b`）・同じ対は `FieldDuplicated`（`pairs.N`））。`pairs` は必須（対が無くても空のリストを送る）。対は転送円どうし・橋どうしで、離れていてよく、相手のいない転送円・橋も保存できる。結果は `ChangeBoardLayoutSuccess` / `ValidationErrors` / `BoardNotFoundError` / `BoardNotDraftError` / `RevisionConflictError` / `ForbiddenError` |
| `changeBoardDefaultRules` | 準備中のボードの既定のルール（目標の魔力 `targetMagic`・開始時の魔力 `startingMagic`・周回のボーナス `lapBonus`・チェックポイントのボーナス `checkpointBonus`・ラウンドの上限 `roundLimit`・サイコロの目の最小 `diceMin`・最大 `diceMax` の 7 つの全体）を変える。レイアウトとは別の操作。同じ値なら revision も進まない。決まりは `board.NewDefaultRules` の 1 か所（目標の魔力・ラウンドの上限・サイコロの目の最小は 1 以上、開始時の魔力・ボーナス 2 つは 0 以上の整数、目標の魔力は開始時の魔力より大きく、サイコロの目の最大は最小以上。上限は持たない）で、違反は入力の項目のパスの `ValidationErrors`（下限を割ったものは `FieldTooSmall{min}`、目標の魔力が開始時の魔力以下は `targetMagic` の `FieldNotGreaterThan{than: "startingMagic"}`、最大が最小より小さいのは `diceMax` の `FieldLessThan{than: "diceMin"}`。等しいのは誤りでないので `FieldNotGreaterThan` と型を分けた）。結果は `ChangeBoardDefaultRulesSuccess` / `ValidationErrors` / `BoardNotFoundError` / `BoardNotDraftError` / `RevisionConflictError` / `ForbiddenError` |
| `deleteBoard` | 準備中のボードを削除する（名前はほかのボードが使えるようになる）。結果は `DeleteBoardSuccess`（id）/ `BoardNotFoundError` / `BoardNotDraftError` / `RevisionConflictError` / `ForbiddenError` |
| `publishBoard` | ボードを公開中にする（準備中・使用不可から。すでに公開中なら変わらない）。公開の条件は、拠点がちょうど 1 つ・チェックポイントが 1 つ以上・すべてのマスに拠点からつながりと対をたどって行けること（対の 2 マスは隣どうしと同じに道として数える）・すべての転送円・橋に対の相手がいること。満たさなければ `BoardNotPublishableError`（`problems`。`Board.publishProblems` と同じ理由ごとの型と座標で、順は 拠点の数 → チェックポイント → 行けないマス → 相手のいない転送円・橋（`UnpairedTilesProblem`）。行けないマスは拠点がちょうど 1 つのときだけ判定し、相手のいない転送円・橋は拠点の数によらず判定する）。結果は `PublishBoardSuccess` / `BoardNotFoundError` / `BoardNotPublishableError` / `RevisionConflictError` / `ForbiddenError` |
| `retireBoard` | ボードを使用不可にする（公開中から。すでに使用不可なら変わらない）。結果は `RetireBoardSuccess` / `BoardNotFoundError` / `InvalidBoardStatusTransitionError` / `RevisionConflictError` / `ForbiddenError` |
| `npcAvatars`（クエリ） | NPC のアバターの一覧（名前の順。`filter.nameContains`・`filter.statuses` で絞り込み、`first`・`after` でページ分け）。結果は `NpcAvatarConnection` / `ValidationErrors` / `ForbiddenError` |
| `npcAvatar`（クエリ） | NPC のアバターの詳細（公開用の ID で指す。名前・状態・使う NPC のデッキ（`decks`。割り当てた順。並べ替えだけでは変わらない）・revision・登録と更新の日時）。結果は `NpcAvatar` / `NpcAvatarNotFoundError` / `ForbiddenError` |
| `registerNpcAvatar` | NPC のアバターを名前だけで登録する（準備中。名前は 20 文字まで）。結果は `RegisterNpcAvatarSuccess`（id・revision）/ `ValidationErrors` / `NpcAvatarNameAlreadyInUseError` / `ForbiddenError` |
| `renameNpcAvatar` | 準備中の NPC のアバターの名前を変える。結果は `RenameNpcAvatarSuccess` / `ValidationErrors` / `NpcAvatarNotFoundError` / `NpcAvatarNameAlreadyInUseError` / `NpcAvatarNotDraftError` / `RevisionConflictError` / `ForbiddenError` |
| `setNpcAvatarDecks` | 準備中の NPC のアバターの使う NPC のデッキを、公開用の ID の一覧（`deckIds`）に置き換える（0 件でもよい。新しく入れるデッキは公開中のものだけ。同じデッキの集まりなら変わらない）。入力の誤りは `ValidationErrors`（項目名は `deckIds.<番号>`）。結果は `SetNpcAvatarDecksSuccess` / `ValidationErrors` / `NpcAvatarNotFoundError` / `NpcAvatarNotDraftError` / `RevisionConflictError` / `ForbiddenError` |
| `deleteNpcAvatar` | 準備中の NPC のアバターを削除する（名前はほかの NPC のアバターが使えるようになる）。結果は `DeleteNpcAvatarSuccess`（id）/ `NpcAvatarNotFoundError` / `NpcAvatarNotDraftError` / `RevisionConflictError` / `ForbiddenError` |
| `publishNpcAvatar` | NPC のアバターを公開中にする（準備中・使用不可から。すでに公開中なら変わらない。使うデッキが 1 つ以上あり、そのうち 1 つ以上が公開中であることが条件で、満たさなければ `ValidationErrors`（項目名は `decks`））。結果は `PublishNpcAvatarSuccess` / `ValidationErrors` / `NpcAvatarNotFoundError` / `RevisionConflictError` / `ForbiddenError` |
| `retireNpcAvatar` | NPC のアバターを使用不可にする（公開中から。すでに使用不可なら変わらない）。結果は `RetireNpcAvatarSuccess` / `NpcAvatarNotFoundError` / `InvalidNpcAvatarStatusTransitionError` / `RevisionConflictError` / `ForbiddenError` |
| `npcDecks`（クエリ） | NPC のデッキの一覧（名前の順。`filter.nameContains`・`filter.statuses` で絞り込み、`first`・`after` でページ分け）。結果は `NpcDeckConnection` / `ValidationErrors` / `ForbiddenError` |
| `npcDeck`（クエリ） | NPC のデッキの詳細（公開用の ID で指す。名前・状態・カードと枚数・合計・写し元・revision 等）。結果は `NpcDeck` / `NpcDeckNotFoundError` / `ForbiddenError` |
| `registerNpcDeck` | NPC のデッキを名前だけで登録する（空のカードで、準備中）。結果は `RegisterNpcDeckSuccess`（id・revision）/ `ValidationErrors` / `NpcDeckNameAlreadyInUseError` / `ForbiddenError` |
| `registerNpcDeckFromPresetDeck` | プリセットデッキ（公開用の ID で指す）からカードと枚数を写して、指定した名前で NPC のデッキを登録する（準備中）。結果は `RegisterNpcDeckFromPresetDeckSuccess`（id・revision）/ `ValidationErrors`（名前）/ `PresetDeckNotFoundError` / `NpcDeckNameAlreadyInUseError` / `ForbiddenError` |
| `renameNpcDeck` | 準備中の NPC のデッキの名前を変える。結果は `RenameNpcDeckSuccess`（id・新しい revision）/ `ValidationErrors` / `NpcDeckNotFoundError` / `NpcDeckNameAlreadyInUseError` / `NpcDeckNotDraftError` / `RevisionConflictError` / `ForbiddenError` |
| `setNpcDeckCardCounts` | 準備中の NPC のデッキのカードの枚数を変える（一覧のどれか 1 つでも誤りがあれば何も変えない）。結果は `SetNpcDeckCardCountsSuccess` / `ValidationErrors` / `NpcDeckNotFoundError` / `NpcDeckNotDraftError` / `RevisionConflictError` / `ForbiddenError` |
| `deleteNpcDeck` | 準備中の NPC のデッキを削除する（名前はほかの NPC のデッキが使えるようになる）。結果は `DeleteNpcDeckSuccess`（id）/ `NpcDeckNotFoundError` / `NpcDeckNotDraftError` / `RevisionConflictError` / `ForbiddenError` |
| `publishNpcDeck` | NPC のデッキを公開中にする（準備中・使用不可から。すでに公開中なら変わらない）。合計がちょうど決まった枚数でない・入っているカードに公開中でないものがある場合は `ValidationErrors`（項目 `cards`）。公開中の数の上限は無い。結果は `PublishNpcDeckSuccess` / `ValidationErrors` / `NpcDeckNotFoundError` / `RevisionConflictError` / `ForbiddenError` |
| `retireNpcDeck` | NPC のデッキを使用不可にする（公開中から。すでに使用不可なら変わらない）。結果は `RetireNpcDeckSuccess` / `NpcDeckNotFoundError` / `InvalidNpcDeckStatusTransitionError` / `RevisionConflictError` / `ForbiddenError` |

カードは状態（`CardStatus`: 準備中 `DRAFT`・公開中 `PUBLISHED`・使用不可 `RETIRED`）を持ち、登録したときは準備中。準備中は自由に変えられ、プリセットデッキに入れられず、`make publish-master` で game に写さない。公開中はプリセットデッキに入れられ、game に写す。使用不可は新しくプリセットデッキに入れられないが、プレイヤーが持っているので game には写す。`cards` の一覧は `filter.statuses` で状態を絞り込め、`Card.status`・`Card.revision` を返す。プリセットデッキに新しく入れる・枚数を増やすカードが公開中でなければ、`setPresetDeckCardCounts` は `FieldUnavailable`（入力の誤り）を返す。すでに入っているカードは、使用不可になっても残せる。

スキーマは共通部分（`common.graphqls`）と機能ごとのファイル（Query・Mutation を `extend` する）に分ける。操作の結果は `<操作>Result = 成功の型 | エラー型` の union で返し（成功の型は、ミューテーションは `<操作>Success` で包み、クエリは問い合わせたもの。1 件なら対象の型、一覧なら 1 ページの `XxxConnection`）、ドメインエラーは GraphQL の `errors` ではなく型で表す（呼び出し側が分岐を型で漏れなく書けるため。設計メモ）。未認証は操作の結果ではないため union に含めず、HTTP 401 で返す。

- **ボード**: 1 ボード 1 集約 `board`（studio のサービス。core#48）。形は四角形の格子（`square` のみ）で、マスは座標（x が列、y が行。0 始まり）と種類（土地 `land`・拠点 `base`・チェックポイント `checkpoint`・転送円 `warp`・橋 `bridge`・聖堂 `sanctuary`・ほこら `shrine`・占い館 `prophecy_shop`・カード屋 `card_shop`。転送円から後ろの 6 つは属性を持たない特殊なマスで、動きは対戦を作るときに作る。施設の名前は仮）、土地だけは属性（火 `fire`・地 `earth`・水 `water`・風 `wind`・無 `neutral`・複 `multi` のどれか 1 つ。複は火・地・水・風の 4 色のマークが描かれた特殊な土地で、組み合わせは持たない。拠点・チェックポイントは持たない。効き方は対戦の決まりなので決めていない）、つながりは向きの無い、上下左右に隣り合うマスどうしだけ。対 `board.Pair` は転送円どうし・橋どうしを組む向きの無い座標の組（つながりと同じく並べ方の小さい方が a で、型は分ける）で、離れていてよい。対はマスに相手を持たせず Layout が集合で持つ（両端に同じ情報を書くと食い違いの検査が要るため）。大きさの範囲は `studio_rules` の `board.min_size`・`board.max_size`（既定は 1〜32。幅・高さに共通。範囲は書くとき（登録・レイアウトの変更）にだけ確かめ、保存済みのイベントを読み直すときは確かめないので、範囲を変えても既存のボードは読める。準備中のボードは、保存し直すときに新しい範囲で確かめる）で、登録の直後は 8×8（範囲の外なら範囲に収めた値。8 はドメインの定数）・マスもつながりも無い。対戦の既定のルール `board.DefaultRules`（目標の魔力 `target_magic`＝届いて拠点に戻ると勝ち・開始時の魔力 `starting_magic`・周回のボーナス `lap_bonus`＝拠点に戻ったとき・チェックポイントのボーナス `checkpoint_bonus`＝通ったとき・ラウンドの上限 `round_limit`＝達したらその時点で魔力の多い人が勝つ・サイコロの目の最小 `dice_min`・最大 `dice_max`。対戦を始めるときにここから変えられる前提の既定の値で、効き方は対戦の決まりなので決めていない。名前の「魔力」は、世界観で呼び方を変えても画面の文言だけで済むよう、core では `magic` と汎用的にした）は、登録のイベントが値を明示して持ち（既定の値 8000・600・300・100・30・1・6 はドメインの定数なので、変えても過去のボードは変わらない）、レイアウトとは別のイベント `DefaultRulesChangedEvent`（変えた後の 7 つの値）で準備中のときだけ変える（同じ値ならイベントを作らない）。値の決まり（目標の魔力・ラウンドの上限・サイコロの目の最小は 1 以上、ほかは 0 以上の整数、目標の魔力は開始時の魔力より大きく、サイコロの目の最大は最小以上。上限は持たない）は書くときの `board.NewDefaultRules` が守り、保存済みのイベントを読み直すときは確かめない（大きさの範囲と同じ。ボードの名前の `board.min_size` のような `studio_rules` には入れていない）。studio_rules の `BoardRules`（大きさの範囲）と名前が重ならないよう、GraphQL の型は `BoardDefaultRules`。状態（準備中・公開中・使用不可）の移り方と、名前の変更・レイアウトの変更・削除が準備中だけなのは配布デッキと同じ。名前はすべてのボードの中で一意（`board.name`）。レイアウトの決まり（マスは格子の中で 1 座標に 1 つ、土地には属性があり拠点・チェックポイントには無い、つながりの両端は置いてあるマスで隣どうし、同じつながりは 1 つ、対の両端は置いてある転送円どうしか橋どうしで自分自身とは組まず、同じマスは 1 つの対にだけ入り、同じ対は 1 つ）は準備中でも保存の時点で守り、違反は `board.ValidateLayout` の 1 か所で数えて、application 層が入力の項目のパスに写す。公開の条件（拠点 1・チェックポイント 1 以上・全マスに拠点からつながりと対をたどって行ける・全転送円と橋に相手がいる。行き止まりは認める）は公開するときの `Decide` で確かめる。レイアウトの変更は変えた後の全体を受け取り、`Decide` が前との差分を 1 つのイベントにする。read model は `boards`（購読者 1009）で、マスとつながりと対（`pairs`）と既定のルール（`default_rules`）は `metadata` に持つ。`make publish-master` は公開中・使用不可の行を core_game の `boards` に写し（準備中は写さない）、game が公開中だけを読む（`Query.boards`・`Query.board`）
- **studio_rules**: studio でマスタを作るときの決まり（今はボードの幅・高さの範囲だけ）は、1 つの集約 `studio_rules`（studio のサービス。一意な key `studio_rules.singleton` で全体に 1 つ。core#52）が値オブジェクト `studiorules.Rules`（internal/domain/studiorules。何の決まりかで `board` に分け、JSON は `{"board": {"min_size": 1, "max_size": 32}}`）で持つ。値の形として確かめるのは、下限が 1 以上・上限が下限以上だけ。`game_rules` と同じく studio が起動時に既定の値で作り（あれば何もしない。複数台で先を越されても成功）、値の変更の操作はまだ無い（core#23 と一緒に作る）。ボードの操作は集約をイベントストアで読んで範囲を確かめ（`StudioRulesHandler.Get`）、ドメインの `Decide` は既定の値を直接読まず、コマンドを動かす側が渡した範囲を使う。read model は `studio_rules`（購読者 1010。表は 1 行）で、`Query.studioRules` が読む。game では使わないので `make publish-master` では写さない（マイグレーションは両方の DB に流すので、core_game にも空の表ができる）。名前の長さは game の画面にも響くため入れない
- **NPC のアバター**: 対戦の相手になるコンピューターのキャラクターは、1 アバター 1 集約 `npc_avatar`（studio のサービス。core#61）。今は名前・状態と、使う NPC のデッキの一覧（core#87）を持ち（見た目・説明・強さは後で足す）、NPC のデッキ（`npc_deck`）は別の集約で、アバター側が持つ（デッキ側は変えない。同じデッキを複数のアバターが使える。参照 `npc_avatar.deck`）。使うデッキは準備中のあいだだけ変えられ、割り当てられるのは公開中のデッキだけ。名前は 20 文字まで（`npcavatar.MaxNameLength`。プレイヤーのアバター `avatar.MaxNameLength` と今は同じ長さだが、型は分けて別々に変えられるようにした）で、すべての NPC のアバターの中で一意（一意な key `npc_avatar.name`。比べ方は `domain.NormalizeName`。登録・名前の変更で確保し、削除の識別子の削除で外れる。プレイヤーのアバターの名前とは別の key）。状態（準備中・公開中・使用不可）の移り方と、名前の変更・削除が準備中だけなのはボードと同じ（`npcavatar.Status`。型は共有しない）。公開するときは、使うデッキが 1 つ以上あり、そのうち 1 つ以上が公開中であることを確かめる（公開中の数の上限は持たない）。変える・消す操作は revision を受け取る。read model は `npc_avatars`（購読者 1011）で、`Query.npcAvatars`・`Query.npcAvatar` が読む。game には `make publish-master` で、公開中・使用不可のアバターだけを写し、game は公開中だけを読む（core#63）
- **NPC のデッキ**: 対戦の相手になる NPC が使うデッキは、1 デッキ 1 集約 `npc_deck`（studio のサービス。core#62）。NPC のアバターから使われる（アバター側が持ち、デッキ側は変えない。core#87）。作り方は配布デッキと同じで、空から作るか、プリセットデッキから写して作る（カード・枚数を写し、名前は指定する。写した元の公開 ID と revision を記録として持つ。元は参照にしない）。名前は NPC のデッキどうしで一意（一意な key `npc_deck.name`。状態によらない）で、長さと比べ方はデッキの名前と同じ。配布デッキ・プリセットデッキと同じ名前でもよい。状態（準備中・公開中・使用不可）の型と移り方は配布デッキと同じ（`npcdeck.Status`。型は共有しない）。名前・枚数の変更と削除は準備中だけ。公開するときは、合計がちょうど `deck.total_card_count`・入っているカードがすべて公開中であることを確かめ、公開中の数の上限は持たない。入れたカードは参照 `npc_deck.card` で持つ。変える・消す操作は revision を受け取る。read model は `npc_decks`（購読者 1012）で、`Query.npcDecks`・`Query.npcDeck` が読む。game には `make publish-master` で、写したアバターに割り当てられたデッキだけを写す（どのアバターにも使われないデッキは写さない。NPC のデッキは単独では使わず、アバターを通してだけ使うため。core#63）。game は公開中のデッキだけを、アバターを通して読む
- **層の分担**: presentation 層（GraphQL）は application 層の型だけを使い、ドメインの型（`operator.ID`・`card.Name`・集約の状態・ドメインのエラー）を知らない。application 層が窓口になり、素の値を受け取ってドメインの型に変え、application 層の結果の型とエラーを返す（ドメインを変えても presentation 層に及ばないように。参照の api は application 層が GraphQL の型を受け渡していたが、そうしない）
- **認可**: オペレーターを要する操作は、resolver の最初に紐づいたオペレーターかを確かめ（`requireOperator`。application 層の `authentication.HasOperator`）、違えば `ForbiddenError` を返す（core#10 でパーミッションの判定に広げる）。操作した人は、application 層のコマンドが context から取り出す
- **入力の誤り**: application 層が `input.Errors`（internal/service/studio/application/input。理由ごとの型 `input.Required`・`input.TooLong{Max}`・`input.OutOfRange{Min, Max}`・`input.TooSmall{Min}`（上限の無い値の下限）・`input.NotGreaterThan{Than}`（別の項目より大きくない）・`input.InvalidFormat` の一覧）を返し、resolver が `ValidationErrors`（`FieldError` のインターフェースと、理由ごとの型 `FieldRequired`・`FieldTooLong{max}`・`FieldOutOfRange{min, max}`・`FieldTooSmall{min}`・`FieldNotGreaterThan{than}`・`FieldInvalidFormat`）に写す。理由によって null になる項目は作らない。文言は返さず、studio が `__typename` と値から作る（上限などの値も core から受け取り、studio に書き写さない）
- **DB のエラーのログ**: 重複（一意性の表）や版の衝突（イベントストア）を、一意制約違反のエラーで受けない。PostgreSQL がふだんの重複のたびに ERROR のログを出し（重複は異常ではないのにログが埋まる）、トランザクションも中断されるため。一意性の表は参照の api と同じく `INSERT ... SELECT ... WHERE NOT EXISTS (ほかの ID が同じ値) ON CONFLICT (id) DO UPDATE ... RETURNING` で、登録にも変更にも使い、行が返らなければ重複とする（まれな同時の競合による一意制約違反も重複として返す）。イベントストアは `ON CONFLICT DO NOTHING RETURNING` で、行が返らなければ版の衝突とする
- **カード名**: Creature・Item・Spell で共有し、前後の空白を除いて NFKC で正規化した値で、3 種類をまたいで重複させない（一意な key の種別 `card.name`。表示は入力どおり）。上限は 50 文字（正規化する前の値で数える）
- **一意性の制約**: 制約ごとの専用の表ではなく、1 つの表 domain_unique_keys（key の種別の中で key が一意）で持つ。何を一意にするか（key の種別と key の作り方）はドメインが宣言し、インフラは確保の仕組みだけを持つ（制約を足すたびに表・SQL を足さないため。1 つの表で全集約のイベントを持つ domain_events と同じ考え方）。210 万件での計測では、専用の表と比べて引く・確保する速さの差は誤差の範囲で、容量は key の種別を数の ID にして抑えた（core#11）。中身は確認用の VIEW `domain_unique_key_view` で読める（列は unique_key_type_id・key・entity_id）
- **数の上限の制約**: 一意性・参照と並ぶ 3 つ目の制約の表 domain_quotas（上限の種別 × 持ち主ごとに使っている数）と domain_quota_types（core#39）。デッキは 1 人にゲーム全体のルール（`game_rules`）の `player.max_decks`（既定は 10 個）まで（種別 `deck.per_player`）、アバターは 1 人に `player.max_avatars`（既定は 5 体）まで（種別 `avatar.per_player`）。確保は作るコマンドと同じトランザクションの 1 つの文（`INSERT ... ON CONFLICT DO UPDATE SET used = used + 1 WHERE used < 上限 RETURNING`）で、上限なら行が返らず `ErrLimitReached` とする（エラーにせず、トランザクションも中断しない）。同時に取り合っても DB が同じ行を順番に更新するので上限を超えず、流し直しは要らない。上限の値は表にも CHECK にも持たず文に渡す（下限の 0 以上だけ CHECK）。使っている数はイベントから作り直せない、書く側の整合性のための記録。上限の値は game の `application/rules` が、写した `game_rules` から読んで返す（まだ写されていなければ、既定の値には戻さずエラー。調整は core#23）。デッキの名前は持ち主ごとに一意（`deck.name`）で、比べ方は `domain.NormalizeName`（カード名・プリセットデッキと同じ）。中身は確認用の VIEW `domain_quota_view` で読める

## game 用の GraphQL（/game/graphql）

スキーマは `internal/service/game/presentation/graphql/*.graphqls`（game は `app schema game` の出力を参照する）。操作は `registerPlayer`・`me`・`registerAvatar`・`boards`・`board`・`npcAvatars`・`npcAvatar`（core の版は、GraphQL ではなく HTTP の `/version` で返す）。

- **registerPlayer**（ミューテーション）: JWT の (iss, sub) のプレイヤーを登録し、公開 ID（`player { id }`）を返す。何度呼んでも、プレイヤーは (iss, sub) ごとに 1 人だけで、登録済みなら何も作らず同じ形（同じ id）で返す。同時に呼ばれても 1 人だけ
- **me**（クエリ）: 自分のプレイヤーを返す。`starterGrant` は read model `players` から読み、行がまだ無い（投影の遅れ）・配布が始まっていないときも `IN_PROGRESS`（画面にとっては「まだ届いていない」と同じ）。`registerPlayer` の結果の `player` からも取れる。未登録なら `PlayerNotRegisteredError`。game は、これが返ったときに `registerPlayer` を呼び、`me` を呼び直す。`avatars` は自分のアバターを作った順に返す（read model `avatars` から読む。上限があるので、ページ分けしないリスト。作った直後は投影の遅れで、まだ出ないことがある）。`decks` は自分のデッキを作った順に返す（read model `decks` から読む。`id`・`name`・`totalCount`・`typeCounts { creature item spell }`（種類ごとの枚数の合計。入っていない種類は 0。画面で数えないよう core が数える）・`cards { count card { id name type } }`。`cards` は種類 → 名前の順で、名前・種類は写した `cards` から引く。写していないカードはエラー。スターターの配布で作られるため、配られた直後は投影の遅れで、まだ出ないことがある。ほかのプレイヤーのデッキは出ない）
- **registerAvatar**（ミューテーション）: ログイン中のプレイヤーのアバターを名前（`input.name`）で作り、作ったアバター（`avatar { id name }`）を返す。結果は `RegisterAvatarSuccess` / `ValidationErrors`（名前が空 `FieldRequired`・20 文字を超える `FieldTooLong{max}`）/ `AvatarNameAlreadyInUseError`（同じプレイヤーの別のアバターが同じ名前。比べ方は `domain.NormalizeName`。ほかのプレイヤーとは同じ名前でよい）/ `AvatarLimitReachedError`（1 人 5 体まで。`max` に上限の値を持つ）/ `PlayerNotRegisteredError`（未登録）。上限は `game_rules` の `player.max_avatars`（種別 `avatar.per_player`）で、写されていなければ作れない。名前の変更・削除はまだ無い
- **boards**（クエリ）: 公開中のボードの一覧（`first`・`after` でページ分け。並びは名前の正規化 → 公開 ID で studio と同じ）。各要素が `id`・`name`・`width`・`height`・`tiles { __typename x y ... on LandTile { attribute } }`（`BoardTile` の interface で、`LandTile`・`BaseTile`・`CheckpointTile` を `__typename` で見分ける。`attribute` は `LandTile` だけが持ち、`FIRE`・`EARTH`・`WATER`・`WIND`・`NEUTRAL`・`MULTI`。行 → 列の順）・`connections { a { x y } b { x y } }`（向きは無い。a → b の順）・`defaultRules { targetMagic startingMagic lapBonus checkpointBonus roundLimit diceMin diceMax }`（対戦を始めるときの既定の値）・`tileCounts { land base checkpoint fire earth water wind neutral multi }`（マスの数）を持つ。状態・revision・日時は持たない。結果は `BoardsConnection` / `ValidationErrors`（`first` が範囲外 `FieldOutOfRange{min, max}`・`after` が読めない `FieldInvalidFormat`）/ `PlayerNotRegisteredError`
- **board**（クエリ）: 公開中のボードの詳細（公開用の ID で指す）。結果は `Board` / `BoardNotFoundError`（無い・準備中・使用不可を区別せず同じ扱い）/ `PlayerNotRegisteredError`。ボードは `make publish-master` で写したもの（core_game の `boards`）を読む
- **gameRules**（クエリ）: ゲーム全体のルールのうち対戦の決まり（`match.minPlayers`・`match.maxPlayers`・`match.maxHandCards`）。`make publish-master` で写した `game_rules` を読み、写されていなければ既定の値に戻さず GraphQL の errors にする。結果は `GameRules` / `PlayerNotRegisteredError`
- **deck**（クエリ）: 自分のデッキの詳細（公開用の ID で指す。`me.decks` の 1 件と同じ項目）。結果は `Deck` / `DeckNotFoundError`（無い・ほかのプレイヤーのデッキを区別せず同じ扱い。存在を明かさない）/ `PlayerNotRegisteredError`
- **npcAvatars**（クエリ）: 公開中の NPC のアバターの一覧（`first`・`after` でページ分け。名前の正規化 → 公開 ID の順）。各要素が `id`・`name`・`decks { id name totalCount cards { count card { id name type } } }` を持つ。`decks` は割り当てた順で、公開中のデッキだけ（準備中・使用不可は出さない）。`cards` は種類（`CREATURE` → `ITEM` → `SPELL`）→ 名前の順。結果は `NpcAvatarsConnection` / `ValidationErrors` / `PlayerNotRegisteredError`。1 ページ分のデッキとカードは、アバターの数によらず 1 回ずつ読む。`make publish-master` で写したもの（core_game の `npc_avatars`・`npc_decks`・`cards`）を読む（core#63）
- **npcAvatar**（クエリ）: 公開中の NPC のアバターの詳細（公開用の ID で指す）。結果は `NpcAvatar` / `NpcAvatarNotFoundError`（無い・準備中・使用不可を区別せず同じ扱い）/ `PlayerNotRegisteredError`。NPC のデッキだけを引く操作は無い（アバターを通してだけ出す）
- **認可**: 登録済みのプレイヤーが要る操作（`registerAvatar`・`boards`・`board`・`npcAvatars`・`npcAvatar` 等）は、resolver の最初に登録済みかを確かめ（`requirePlayer`）、未登録なら `PlayerNotRegisteredError` を返す
- **入力の誤り**: studio 用と同じ形（application 層が `input.Errors`（internal/service/game/application/input）を返し、resolver が `ValidationErrors` に写す）。理由は game で要るもの（`Required`・`TooLong`・`OutOfRange`・`InvalidFormat`）から足す

- **認証**: studio 用と同じく、すべての呼び出しに game（better-auth の jwt プラグイン）の JWT を付ける。検証（署名・`iss`・`aud`・期限）、JWKS の取り直し、401 の返し方、POST だけを受け付けることも studio 用と同じ。JWKS は game-envoy を通して取る
- **発行元の区別**: `aud` は studio 用と同じ `diecardomain-core` で、`iss`（`diecardomain-game`）で発行元を区別する。studio の JWT は game の入口では通らない（逆も同じ）
- **DB**: game の入口の操作は、game 用の DB（core_game）だけを読み書きする（core#33）
- **ゲーム全体のルール**: デッキの合計の枚数・同じカードの枚数・持てる数の上限など、運営が決める全体の値は、1 つの集約 `game_rules`（studio のサービス。一意な key `game_rules.singleton` でゲーム全体に 1 つ）が持つ（core#43）。値は値オブジェクト `gamerules.Rules`（internal/domain/gamerules。何の決まりかで `player`・`deck`（デッキに共通）・`starter_deck`・`match`（対戦の決まり。参加人数の範囲 `min_players`〜`max_players`（既定は 2〜4）と手札の上限 `max_hand_cards`（既定は 6）。人数は下限が上限以下。盤面や部屋によらず全体で同じ。core#65）に分け、JSON はスネークケース）で、studio と game が同じ型を使う。studio が起動時に既定の値で作る（あれば何もしない。複数台で先を越されても成功）。値の変更の操作はまだ無い（core#23）。studio は集約をイベントストアで読んで確かめる（投影の遅れで古い値を見ないため）。game は read model `game_rules` を `make publish-master` で写したものを読み、写されていなければ既定の値に黙って戻さずエラー（配布のポリシーでは、写されれば直る失敗）。カード名・デッキ名の長さは値の形の決まりとして `game_rules` に入れない
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
| マイルストーンのブランチに向かう MR（Issue の MR） | check（手動） | ふだんは実行せず、手元の `make lint`・`make test` で確かめる。パイプラインは skipped になり、「スキップされたパイプラインを成功とみなす」の設定でマージできる。CI の手順を変えたとき等、CI で確かめたいときだけ手で動かす（失敗するとマージできなくなる） |
| main に向かう MR（マイルストーンの MR） | check | `make lint` → `make test` 相当を 1 つのジョブで順に流す（GitLab のコンピューティング時間を短縮するため、ジョブごとの準備を 1 回にする。lint で落ちたら test は流さない。core#79）。Go のモジュールとビルドの結果を GitLab の cache（`.cache-ci`。鍵は go.sum と GO_VERSION）に持ち、毎回のダウンロードとコンパイルを省く（core#78） |
| main に向かう MR | build-mr（手動） | `registry.gitlab.com/aimerzarashi/diecardomain/core:mr-<MR番号>` と `.../core/migrate:mr-<MR番号>` に push |
| main | release | semantic-release がコミットからバージョンを決め、タグ（`v1.2.0`）と GitLab リリースを作る。リリース対象のコミットが無ければ何もしない。入った MR・Issue へのコメント（どの版で入ったか）は書かない（`.releaserc.json` の `successCommentCondition: false`。書くと GitLab の回数の制限にかかることがあるため。core#77） |
| タグ（`v*`） | deploy | Docker Hub の `diecardomain/core:<タグ>`・`:latest` と `diecardomain/core-migrate:<タグ>`・`:latest` に push |

バージョンは Conventional Commits で決まる（`feat:` → マイナー、`fix:` / `perf:` → パッチ、`BREAKING CHANGE` → メジャー）。

マイルストーンのブランチに向かう Issue の MR では check のジョブを手動にして、ふだんは実行しない（手元の `make lint`・`make test` で確かめる。GitLab のコンピューティング時間を Issue ごとに使わないため）。手動のジョブだけのパイプラインは skipped になり、「スキップされたパイプラインを成功とみなす」の設定でマージできる（「パイプラインが成功する必要がある」の設定は、main に向かう MR の歯止めとして残す）。main への push では lint・test を流さない（直前の main に向かう MR で同じ中身を確かめているため）。main は直接 push できない設定（保護ブランチの push は No one）にして、すべての変更が main に向かう MR の check を通るようにしている（core#30・core#33）。

## カスタムリンター（tools/linter）

参照プロジェクトの api のカスタムリンターを移植したもの（#8）。`make lint` と CI の check のジョブ（lint の手順）で `go run ./tools/linter ./...` を実行する。対象の設定は `tools/linter/main.go` に書く（`make lint` と CI で同じ設定にするため）。

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

[Atlas Community Edition](https://atlasgo.io/community-edition)（`arigaio/atlas:1.3.5-community`）で、`migrations/NNNN_name.sql` のうち未適用のものだけを番号順に適用する。
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

Atlas は以下を黙って通す（`migrate validate` / `migrate lint` でも検出しない）ため、`make lint` と CI の check のジョブ（lint の手順）で検査する。
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
| CI | check のジョブ（test の手順）で Atlas（v1.3.5）を取得し、適用してから go test |
| リリース・本番 | タグのパイプラインで `diecardomain/core-migrate:<タグ>` を Docker Hub に push。本番では core より先に、studio 用・game 用の DB それぞれの `DATABASE_URL` を渡してジョブとして実行する |
