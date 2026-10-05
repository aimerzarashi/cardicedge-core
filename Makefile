# 環境管理

.PHONY: up
up:
	docker compose up -d --build --wait

# DB を破棄する（次回 up で migrations が再適用される）。Go のキャッシュ用ボリュームは残す
.PHONY: down
down:
	docker compose rm -sfv core-db
	docker compose down

# 開発ツール

# フォーマットを自動修正する（lint はファイルを書き換えないため、修正はこちらで行う）
.PHONY: fmt
fmt:
	docker compose exec core gofmt -w .

# gofmt（違反ファイルの検出のみ。書き換えない）、go vet、staticcheck
.PHONY: lint
lint:
	docker compose exec core sh -c 'files=$$(gofmt -l .); [ -z "$$files" ] || { echo "gofmt:"; echo "$$files"; exit 1; }'
	docker compose exec core go vet ./...
	docker compose exec core go tool staticcheck ./...

# テスト

.PHONY: test
test:
	docker compose exec core go test ./... -race
