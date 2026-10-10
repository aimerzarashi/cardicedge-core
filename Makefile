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

# migrations/ のファイルを足したら実行し、atlas.sum を作り直す（SQL と一緒にコミットする）
.PHONY: migrate-hash
migrate-hash:
	docker compose run --rm --no-deps --user $$(id -u):$$(id -g) core-migrate migrate hash --dir file:///migrations

# 開発ツール

# フォーマットを自動修正する（lint はファイルを書き換えないため、修正はこちらで行う）
.PHONY: fmt
fmt:
	docker compose exec core gofmt -w .

# gofmt（違反ファイルの検出のみ。書き換えない）、go vet、staticcheck、migrations の検査（origin/main との分岐点からの変更も見る）
.PHONY: lint
lint:
	docker compose exec core sh -c 'files=$$(gofmt -l .); [ -z "$$files" ] || { echo "gofmt:"; echo "$$files"; exit 1; }'
	docker compose exec core go vet ./...
	docker compose exec core go tool staticcheck ./...
	scripts/check-migrations.sh $$(git merge-base origin/main HEAD)

# テスト

.PHONY: test
test:
	docker compose exec core go test ./... -race
