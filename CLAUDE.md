# コマンド

- `make up` / `make down` / `make fmt` / `make lint` / `make test`
- lint は `make lint`（gofmt の検出、go vet、staticcheck）でファイルを書き換えない。フォーマットの修正は `make fmt`。staticcheck は go.mod の tool で管理する
- テストは `make test`（コンテナ内で `-race` 付き）で実行する。ホストには cgo がなく `-race` が使えない
- 単体で動かすときは `docker compose exec core go test ./<pkg> -race -run <Test>`

# ディレクトリ構成

| パス | 責務 |
|---|---|
| cmd/app | エントリポイント。HTTP サーバーの起動とグレースフルシャットダウン |
| internal/platform/rdb | PostgreSQL への接続 |
| migrations | DB マイグレーション（`NNNN_name.sql`、docker-entrypoint-initdb.d で適用） |
| local/core | 開発用 Dockerfile（air によるホットリロード。uid 1000 の dev ユーザーで動く） |

# 実装上の約束

- シャットダウン時間は `shutdownTimeout`（cmd/app） < air の `--build.kill_delay`（compose.yaml） < compose の `stop_grace_period` の順に保つ
