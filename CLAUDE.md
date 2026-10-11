# コマンド

- `make up` / `make down` / `make migrate` / `make migrate-hash` / `make fmt` / `make lint` / `make test`
- lint は `make lint`（gofmt の検出、go vet、staticcheck、カスタムリンター `go run ./tools/linter ./...`、scripts/check-migrations.sh）でファイルを書き換えない。フォーマットの修正は `make fmt`。staticcheck は go.mod の tool で管理する
- テストは `make test`（コンテナ内で `-race` 付き）で実行する。ホストには cgo がなく `-race` が使えない
- 単体で動かすときは `docker compose exec core go test ./<pkg> -race -run <Test>`

# ディレクトリ構成

| パス | 責務 |
|---|---|
| cmd/app | エントリポイント。HTTP サーバーの起動とグレースフルシャットダウン |
| internal/platform/rdb | PostgreSQL への接続 |
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
