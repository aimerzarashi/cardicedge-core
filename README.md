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

## DB マイグレーション

`migrations/NNNN_name.sql` を置くと、PostgreSQL の初回起動時に `docker-entrypoint-initdb.d` の仕組みでファイル名順に適用される。
初回起動時にしか実行されないため、追加・変更したら `make down` → `make up` で DB を作り直す。
