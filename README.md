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
make fmt     # gofmt でフォーマットを修正
make lint    # gofmt・go vet・staticcheck・カスタムリンター（tools/linter）・migrations の検査
make test    # コンテナ内で go test ./... -race を実行
make down    # 停止して DB のデータも破棄
```

| URL | 内容 |
|---|---|
| http://localhost:3333/version | バージョン情報 |
| http://localhost:3333/health | ヘルスチェック |
| http://localhost:3333/cms/graphql | cms 用の GraphQL（POST。cms の JWT が必須） |

PostgreSQL は `localhost:5432`（user: core_user / password: core_password / db: core）で接続できる。

ポートが他と衝突する場合は `CORE_PORT=13333 CORE_DB_PORT=15432 make up` のように変更できる。

## 他プロジェクトとの統合

`make up` すると共有ネットワーク `diecardomain` に参加する（無ければ作成する）。
game / cms は同じネットワーク経由で `http://core:3333` に接続する。core-db は参加しない。

## cms 用の GraphQL（/cms/graphql）

スキーマは `internal/cms/uif/graphql/*.graphqls`（cms はこのファイルを参照する）。

- **認証**: すべての呼び出しに、cms（better-auth の jwt プラグイン）の JWT を `Authorization: Bearer <JWT>` で付ける。無い・不正なら 401（`extensions.code: UNAUTHENTICATED`。理由は返さない）。ネットワークの位置（内部からの呼び出しか）は信じない（game からも core に直接届くため）
- **検証**: 署名（EdDSA に固定。none・HMAC への差し替えを受け付けない）・`iss`・`aud`・期限。JWKS は cms-envoy を通して取る
- **JWKS の取り直し**: 1 時間ごとと、知らない kid の JWT が来たとき（5 分に 1 回まで）。起動時に cms が動いていなくても core は起動する（最初の JWT が来たときに取り直す。そのときも cms が動いていなければ、次は 5 分後）
- **cms の secret が漏れたとき**: cms の手順で jwks を作り直したら、core を再起動して古い鍵をすぐに捨てる（再起動しなくても 1 時間以内の取り直しで消える）
- POST だけを受け付ける（ミューテーションを GET で実行させる CSRF を防ぐため）
- **オペレーターの特定と紐づけ**: JWT の (iss, sub) に紐づいたオペレーターを、認可で使えるように context に載せる。まだ紐づいていなければ、JWT の確認済みのメールアドレス（`email`・`email_verified`。cms が JWT に入れる）で、登録済み・未紐づけのオペレーターを探して紐づける（初めてのログイン）。紐づけた後は (iss, sub) だけで識別し、メールアドレスは使わない。確認していないメールアドレスでは紐づけない（他人のメールアドレスでサインアップしても、オペレーターになれないように）
- JWT が正しければ、オペレーターに紐づかなくても入口では拒否しない。オペレーターかどうか・権限は、操作ごとに認可で判定し、各操作の結果の型（`ForbiddenError`）で返す（最初にオペレーターを要する操作を足すときに作る）

| 環境変数 | 内容 |
|---|---|
| `DB_DSN` | core-db の接続先 |
| `CMS_JWKS_URL` | cms の JWKS（開発・テストは `http://cms-envoy:10000/api/auth/jwks`） |
| `CMS_JWT_ISSUER` / `CMS_JWT_AUDIENCE` | cms の `JWT_ISSUER` / `JWT_AUDIENCE` と一致させる（`diecardomain-cms` / `diecardomain-core`） |
| `CMS_INITIAL_OPERATOR_EMAIL` | 初期オペレーターのメールアドレス（開発・テストは `operator@example.com`）。オペレーターが 1 人もいないときだけ、起動時に登録する。本人がこのアドレスで cms にサインアップし、確認してログインすると紐づく。2 回目以降の起動では、変えても何もしない |

どれも必須で、未設定なら起動しない（既定値で動くと、別の発行元の JWT を受け付ける等の設定の誤りに気づけないため）。

| 操作 | 内容 |
|---|---|
| `version` | core の版（GraphQL の仕様で Query が 1 つ以上要るため置く） |

スキーマは共通部分（`common.graphqls`）と機能ごとのファイル（Query・Mutation を `extend` する）に分ける。操作の結果は `<操作>Result = <操作>Success | エラー型` の union で返し、ドメインエラーは GraphQL の `errors` ではなく型で表す（呼び出し側が分岐を型で漏れなく書けるため。設計メモ）。未認証は操作の結果ではないため union に含めず、HTTP 401 で返す。

## イベントストア

DDD + CQRS + Event Sourcing（設計メモ）。参照プロジェクトの api の形に、設計メモの 6 節の修正点を反映し、さらに次の点を変えた（#7）。

- 種別のレジストリ（`typeregistry.Registry`）はパッケージの変数にせず、起動時に読み込んで `identifier`・`eventstore` に渡す
- 種別名・イベント名は、型が宣言する（ID の `EntityType()`、イベントの `EventType()`）。起動時に `Registry.Verify` で DB（マイグレーション）と照合し、食い違えば起動しない
- イベントのスキーマの版は、イベントの型が宣言する（`SchemaVersion()`）。読み込み時は保存された版を `Unmarshal` に渡す
- 集約を足したら: 種別とイベント種別のマイグレーションを足し（`make migrate-hash`・`make migrate`・`make gen-orm`）、起動時の `Verify` と `typeregistry` のテストにその集約の `Types()` を足す

中身を確かめるときは、種別の id を名前に展開した確認用の VIEW（`domain_identifier_view`・`domain_event_view`・`domain_event_type_view`）を使う（アプリからは使わない）。

テストで DB を使うときは `rdbtest.Tx`（テストの終わりに rollback し、開発用の DB を汚さない）。commit そのものを確かめるテスト（同時実行等）は `rdbtest.Open` を使い、作った行を消す。

## テスト用環境（compose.test.yaml）

game / cms のテスト用に、使い捨ての core と DB を起動する compose。利用側の compose から include して使う。

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
| MR | lint / test | `make lint` / `make test` 相当 |
| MR | build-mr（手動） | `registry.gitlab.com/aimerzarashi/diecardomain/core:mr-<MR番号>` と `.../core/migrate:mr-<MR番号>` に push |
| main | lint / test / release | semantic-release がコミットからバージョンを決め、タグ（`v1.2.0`）と GitLab リリースを作る。リリース対象のコミットが無ければ何もしない |
| タグ（`v*`） | deploy | Docker Hub の `diecardomain/core:<タグ>`・`:latest` と `diecardomain/core-migrate:<タグ>`・`:latest` に push |

バージョンは Conventional Commits で決まる（`feat:` → マイナー、`fix:` / `perf:` → パッチ、`BREAKING CHANGE` → メジャー）。

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
core・cms・game で同じ形式・同じ実行役にそろえている（方針は #5）。

| ファイル | 内容 |
|---|---|
| `migrations/NNNN_name.sql` | PostgreSQL の SQL（手で書く） |
| `migrations/atlas.sum` | Atlas が作るハッシュの一覧。SQL と一緒にコミットする |
| `local/migrate/Dockerfile`・`atlas.hcl` | マイグレーション用イメージ（Atlas + migrations）。DB の接続先は実行時に `DATABASE_URL` で渡す |

### 追加の手順

1. `migrations/` に、その時点の最大の番号 + 1 で SQL を置く
2. `make migrate-hash` で `atlas.sum` を作り直す
3. `make migrate` で開発用の DB に適用する
4. SQL と `atlas.sum` を一緒にコミットする

### 決まり

- ファイル名は `NNNN_name.sql`（番号は 4 桁の数字、名前は小文字・数字・`_`）。Atlas は最初の `_` までを番号として読む
- 追加のみ。down は無い。誤りは新しいマイグレーションで直す。適用済みのファイルは変更・削除・改名しない
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
適用済みファイルの変更は、比較元からの差分で見る（`make lint` は origin/main との分岐点、CI は MR なら対象ブランチとの分岐点、main なら push 前の先頭。MR を経ない直接 push も検出する）。lint が通らなければ build-mr・release も動かない。

| 検査 | Atlas に任せた場合 |
|---|---|
| 番号の重複 | 後のファイルが適用されないまま status が OK になる。未適用同士の重複は片方だけ適用して panic する |
| 適用済みファイルの変更・削除・改名（分岐点からの差分） | `atlas.sum` を作り直されると apply は何もせず通り、DB とファイルが食い違う |
| 比較元の最大より小さい番号の追加 | 空の DB に全部を適用するテストは通り、本番（適用済みの DB）で初めて out of order で止まる |
| ファイル名の形式 | `0002_5_x.sql` は番号 `0002` と読まれ、重複になる |
| 指示行の直後の空行 | `-- atlas:txmode none` が無視される |

| 環境 | 実行方法 |
|---|---|
| 開発（compose.yaml） | core-migrate サービス（migrations をマウント）が core の起動前に 1 回実行。`make migrate` |
| テスト用（compose.test.yaml） | core-migrate サービス（マイグレーション用イメージ） |
| CI | test ジョブで Atlas（v1.3.3）を取得し、適用してから go test |
| リリース・本番 | タグのパイプラインで `diecardomain/core-migrate:<タグ>` を Docker Hub に push。本番では core より先に `DATABASE_URL` を渡してジョブとして実行する |
