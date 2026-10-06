# core

カルドセプト風ブラウザゲームのバックエンド（Go API）。

## 必要なもの

- Docker Engine + Docker Compose v2 以降
- make

想定環境は Linux / WSL2（ホストの uid 1000）。開発用コンテナは uid 1000 のユーザーで動き、生成物をホストユーザー所有にする。

Go・PostgreSQL はコンテナ内で動くため、ホストへのインストールは不要。

## セットアップ

```bash
make up    # app（air でホットリロード）と PostgreSQL を起動
make fmt   # gofmt でフォーマットを修正
make lint  # gofmt・go vet・staticcheck
make test  # コンテナ内で go test ./... -race を実行
make down  # 停止して DB のデータも破棄
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
| MR | build-mr（手動） | `registry.gitlab.com/aimerzarashi/diecardomain/core:mr-<MR番号>` に push |
| main | lint / test / release | semantic-release がコミットからバージョンを決め、タグ（`v1.2.0`）と GitLab リリースを作る。リリース対象のコミットが無ければ何もしない |
| タグ（`v*`） | deploy | Docker Hub の `diecardomain/core:<タグ>` と `:latest` に push |

バージョンは Conventional Commits で決まる（`feat:` → マイナー、`fix:` / `perf:` → パッチ、`BREAKING CHANGE` → メジャー）。

## DB マイグレーション

`migrations/NNNN_name.sql` を置くと、PostgreSQL の初回起動時に `docker-entrypoint-initdb.d` の仕組みでファイル名順に適用される。
初回起動時にしか実行されないため、追加・変更したら `make down` → `make up` で DB を作り直す。
