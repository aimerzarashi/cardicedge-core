# 環境管理

# 共有ネットワーク diecardomain が無ければ作成してから起動する
.PHONY: up
up:
	@docker network inspect diecardomain >/dev/null 2>&1 || docker network create diecardomain >/dev/null
	docker compose up -d --build --wait

# 停止して DB を破棄する（次回 up で migrations が最初から適用される）。Go のキャッシュ用ボリュームは残す
.PHONY: down
down:
	docker compose rm -sfv core-db
	docker compose down

# 未適用のマイグレーションだけを Atlas で適用する（DB を作り直さずに、追加した migrations を反映する）
.PHONY: migrate
migrate:
	docker compose run --rm core-migrate

# ユニットテストが複製するテンプレート（core_template）を、無ければ作り、マイグレーションを流す（make test の前に必ず実行する）。
# テンプレートは core と分けるのは、アプリやテストが書き込む DB を複製すると、書き込みの途中を写しうるため
.PHONY: migrate-template
migrate-template:
	docker compose exec core go run ./tools/createdb -name core_template
	docker compose run --rm -e DATABASE_URL="postgres://core_user:core_password@core-db:5432/core_template?sslmode=disable" core-migrate

# migrations/ のファイルを足したら実行し、atlas.sum を作り直す（SQL と一緒にコミットする）
.PHONY: migrate-hash
migrate-hash:
	docker compose run --rm --no-deps --user $$(id -u):$$(id -g) core-migrate migrate hash --dir file:///migrations

# 開発ツール

# GraphQL のスキーマ（internal/service/studio/presentation/graphql/*.graphqls）から gqlgen のコードを生成する。生成したコードもコミットする
.PHONY: gen-api
gen-api:
	docker compose exec core sh -c 'cd internal/service/studio/presentation/graphql && go tool gqlgen generate'
	docker compose exec core go mod tidy

# マイグレーション済みの core-db から sqlboiler のモデル（internal/platform/orm/sqlboiler/models）を生成する。
# migrations を足したら make migrate の後に実行し、生成したコードもコミットする。
# 生成の道具は go.mod の tool で版を固定する（go install @latest にしない）。ドライバーは sqlboiler が PATH から探すため、ビルドしてパスで渡す
.PHONY: gen-orm
gen-orm:
	docker compose exec core sh -c 'go build -o /tmp/sqlboiler-psql github.com/aarondl/sqlboiler/v4/drivers/sqlboiler-psql && cd internal/platform/orm/sqlboiler && go tool sqlboiler --config sqlboiler.toml /tmp/sqlboiler-psql'
	docker compose exec core go mod tidy


# フォーマットを自動修正する（lint はファイルを書き換えないため、修正はこちらで行う）
.PHONY: fmt
fmt:
	docker compose exec core gofmt -w .

# gofmt（違反ファイルの検出のみ。書き換えない）、go vet、staticcheck、カスタムリンター（tools/linter）、migrations の検査（origin/main との分岐点からの変更も見る）
.PHONY: lint
lint:
	docker compose exec core sh -c 'files=$$(gofmt -l .); [ -z "$$files" ] || { echo "gofmt:"; echo "$$files"; exit 1; }'
	docker compose exec core go vet ./...
	docker compose exec core go tool staticcheck ./...
	docker compose exec core go run ./tools/linter ./...
	docker compose exec core scripts/check-gen-api.sh
	scripts/check-migrations.sh $$(git merge-base origin/main HEAD)

# テスト
.PHONY: test
test: migrate-template
	docker compose exec core go test ./... -race
