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
make fmt     # gofmt でフォーマットを修正
make lint    # gofmt・go vet・staticcheck・migrations の検査
make test    # コンテナ内で go test ./... -race を実行
make down    # 停止して DB のデータも破棄
```

| URL | 内容 |
|---|---|
| http://localhost:3333/version | バージョン情報 |
| http://localhost:3333/health | ヘルスチェック |

PostgreSQL は `localhost:5432`（user: core_user / password: core_password / db: core）で接続できる。

ポートが他と衝突する場合は `CORE_PORT=13333 CORE_DB_PORT=15432 make up` のように変更できる。

## 他プロジェクトとの統合

`make up` すると共有ネットワーク `diecardomain` に参加する（無ければ作成する）。
game / cms は同じネットワーク経由で `http://core:3333` に接続する。core-db は参加しない。

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
