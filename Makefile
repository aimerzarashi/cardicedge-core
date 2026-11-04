# 環境管理

# 共有ネットワーク（game 用の diecardomain-game・studio 用の diecardomain-studio）が無ければ作成してから起動する
.PHONY: up
up:
	@for network in diecardomain-game diecardomain-studio; do \
	  docker network inspect $$network >/dev/null 2>&1 || docker network create $$network >/dev/null; \
	done
	docker compose up -d --build --wait

# 停止して DB を破棄する（次回 up で migrations が最初から適用される）。Go のキャッシュ用ボリュームは残す
.PHONY: down
down:
	docker compose rm -sfv core-db
	docker compose down

# down に加えて、トレースとログ（otel-lgtm の名前つきのボリューム）と core-db のログのボリュームも消す。down では調べるために残している（core#82・core#83）
.PHONY: clean
clean: down
	docker volume rm -f diecardomain-otel-lgtm-data diecardomain-core-db-log

# 未適用のマイグレーションだけを Atlas で適用する（DB を作り直さずに、追加した migrations を反映する）。
# 種別の行はマイグレーションで入れず、コードの一覧から入れる（core は起動時にも入れるが、テストが core を起動せずに使うため。core#33）
.PHONY: migrate
migrate:
	docker compose run --rm core-migrate-studio
	docker compose run --rm core-migrate-game
	docker compose exec core go run ./tools/synctypes -env STUDIO_DB_DSN
	docker compose exec core go run ./tools/synctypes -env GAME_DB_DSN

# テストが使う DB（core_test）と、複製するテンプレート（core_template）を、無ければ作り、マイグレーションを流して種別の行を入れる
# （make test の前に必ず実行する）。テンプレートを core_test と分けるのは、テストが書き込む DB を複製すると、書き込みの途中を写しうるため
.PHONY: migrate-template
migrate-template:
	docker compose exec core go run ./tools/createdb -env TEST_DB_DSN -name core_test
	docker compose exec core go run ./tools/createdb -env TEST_DB_DSN -name core_template
	docker compose run --rm -e DATABASE_URL="postgres://core_user:core_password@core-db:5432/core_test?sslmode=disable" core-migrate-studio
	docker compose run --rm -e DATABASE_URL="postgres://core_user:core_password@core-db:5432/core_template?sslmode=disable" core-migrate-studio
	docker compose exec core go run ./tools/synctypes -env TEST_DB_DSN
	docker compose exec core go run ./tools/synctypes -env TEST_DB_DSN -dbname core_template

# migrations/ のファイルを足したら実行し、atlas.sum を作り直す（SQL と一緒にコミットする）
.PHONY: migrate-hash
migrate-hash:
	docker compose run --rm --no-deps --user $$(id -u):$$(id -g) core-migrate-studio migrate hash --dir file:///migrations

# 開発ツール

# GraphQL のスキーマ（internal/service/{studio,game}/presentation/graphql/*.graphqls）から gqlgen のコードを生成する。生成したコードもコミットする
.PHONY: gen-api
gen-api:
	docker compose exec core sh -c 'cd internal/service/studio/presentation/graphql && go tool gqlgen generate'
	docker compose exec core sh -c 'cd internal/service/game/presentation/graphql && go tool gqlgen generate'
	docker compose exec core go mod tidy

# 開発で触る見本のマスタ（カード・プリセットデッキ・配布デッキ・ボード・NPC・スターターセット）を、初期オペレーターとして studio のコマンドで登録する。
# 配布デッキはプリセットデッキから写して作って公開する。スターターセットは見本の配布デッキとボードを指定して公開する。作成済みなら飛ばすので、何度流しても同じ結果になる。
# 終わりに core の投影が grant_decks を書くまで待つので、続けて publish-master を流せる（core#37）
.PHONY: seed-master
seed-master:
	docker compose exec core go run ./tools/seedmaster

# マスタ（studio の read model の cards・grant_decks・starter_sets・game_rules・boards・npc_avatars・npc_decks）を game の DB に公開する。core-db の中で、core_studio から \copy で読んだ行を
# そのまま core_game の \copy に渡し、1 つのトランザクションで空にして流し込むので、失敗しても core_game は元のまま。
# cards は公開中・使用不可のカードだけを写す（準備中は studio の中の作りかけで、game に出さない。使用不可はプレイヤーが持っているので写す）。
# pg_dump は行で絞れないため使わない。流し込む前に全体を一時ファイルに作るのは、読む側が途中で失敗しても、書く側が途中までの入力で
# 空にして確定してしまわないため。
# boards は cards と同じ選び方（公開中・使用不可だけ。準備中は写さない。使用不可は game が一覧にも詳細にも出さないが、後で対戦中のボードの元を
# 表示したくなっても表を作り直さずに済むよう写す）
# grant_decks は全行（準備中・使用不可も、状態つき）を写し、game は公開中だけを読む（後で配り済みのデッキの元を表示したくなっても、
# 表を作り直さずに済むように）
# starter_sets も grant_decks と同じく全行を状態つきで写し、game は公開中の 1 つだけを読む（配る配布デッキは、そのセットが指す公開 ID で
# 写した grant_decks の公開中の行から引く。解放するボードは公開 ID のまま持ち、配布のとき game の内部 ID に引き直して解放の集約へ渡す）
# 写した cards・boards・npc_avatars・npc_decks は、game の domain_entities にも行を作る（公開 ID は studio と同じ。内部 ID は game の連番が振る）。game の集約は、
# 境界（studio・game）の中の内部 ID で ほかの集約を指すため、写したボード・NPC のアバター・NPC のデッキ（部屋の席の選択が指す）を game の内部 ID で指せるようにする。
# ON CONFLICT DO NOTHING なので、何度流しても同じ内部 ID は変わらない（TRUNCATE は domain_entities に及ばない）。
# カードの種類（集約の種別）は、写した cards.card_type（1: Creature、2: Item、3: Spell）から決める
# npc_avatars も boards と同じ選び方（公開中・使用不可だけ。game が出すのは公開中だけ）
# npc_decks は、写した npc_avatars の metadata の decks（使うデッキの公開 ID）に入っているものだけを、状態によらず写す（core#63）。
# NPC のデッキは単独では使わず、アバターを通してだけ使う（game が公開中のデッキだけを出す）ため、どのアバターにも使われていないデッキ
# （準備中の作りかけを含む）を game に出す理由が無い。アバターを公開中・使用不可に絞った結果から導くので、写さないアバターのデッキも写らない。
# 状態で絞らないのは、grant_decks と同じく状態つきで写して game が公開中だけを読めばよく、使用不可のデッキも後で表示したくなったときに表を作り直さずに済むため
# イベントの仕組みで game に写さないのは、内部 ID が DB ごとに振られて重なるため。表の写しで足りるのは、この 7 つの表が公開用の ID だけで
# 持ち、外部キーも無く、別の DB に入れても意味が変わらないため。両方の DB が同じマイグレーションまで適用済みであること（core#37）
.PHONY: publish-master
publish-master:
	docker compose exec core-db sh -c 'set -e; f=$$(mktemp); trap "rm -f $$f" EXIT; { \
	  printf "%s\n" "TRUNCATE cards, boards, grant_decks, starter_sets, game_rules, npc_avatars, npc_decks;" "\\copy cards FROM STDIN"; \
	  psql -U core_user -d core_studio -v ON_ERROR_STOP=1 -c "\\copy (SELECT * FROM cards WHERE status IN ('"'"'published'"'"', '"'"'retired'"'"')) TO STDOUT"; \
	  printf "%s\n" "\\." "INSERT INTO domain_entities (public_id, entity_type_id) SELECT c.card_id, t.id FROM cards c JOIN domain_entity_types t ON t.name = CASE c.card_type WHEN 1 THEN '"'"'creature'"'"' WHEN 2 THEN '"'"'item'"'"' WHEN 3 THEN '"'"'spell'"'"' END ON CONFLICT DO NOTHING;" "\\copy boards FROM STDIN"; \
	  psql -U core_user -d core_studio -v ON_ERROR_STOP=1 -c "\\copy (SELECT * FROM boards WHERE status IN ('"'"'published'"'"', '"'"'retired'"'"')) TO STDOUT"; \
	  printf "%s\n" "\\." "INSERT INTO domain_entities (public_id, entity_type_id) SELECT b.board_id, t.id FROM boards b JOIN domain_entity_types t ON t.name = '"'"'board'"'"' ON CONFLICT DO NOTHING;" "\\copy npc_avatars FROM STDIN"; \
	  psql -U core_user -d core_studio -v ON_ERROR_STOP=1 -c "\\copy (SELECT * FROM npc_avatars WHERE status IN ('"'"'published'"'"', '"'"'retired'"'"')) TO STDOUT"; \
	  printf "%s\n" "\\." "INSERT INTO domain_entities (public_id, entity_type_id) SELECT a.npc_avatar_id, t.id FROM npc_avatars a JOIN domain_entity_types t ON t.name = '"'"'npc_avatar'"'"' ON CONFLICT DO NOTHING;" "\\copy npc_decks FROM STDIN"; \
	  psql -U core_user -d core_studio -v ON_ERROR_STOP=1 -c "\\copy (SELECT * FROM npc_decks WHERE npc_deck_id IN (SELECT deck_id::uuid FROM npc_avatars, jsonb_array_elements_text(metadata->'"'"'decks'"'"') AS deck_id WHERE status IN ('"'"'published'"'"', '"'"'retired'"'"'))) TO STDOUT"; \
	  printf "%s\n" "\\." "INSERT INTO domain_entities (public_id, entity_type_id) SELECT d.npc_deck_id, t.id FROM npc_decks d JOIN domain_entity_types t ON t.name = '"'"'npc_deck'"'"' ON CONFLICT DO NOTHING;" "\\copy grant_decks FROM STDIN"; \
	  psql -U core_user -d core_studio -v ON_ERROR_STOP=1 -c "\\copy grant_decks TO STDOUT"; \
	  printf "%s\n" "\\." "\\copy starter_sets FROM STDIN"; \
	  psql -U core_user -d core_studio -v ON_ERROR_STOP=1 -c "\\copy starter_sets TO STDOUT"; \
	  printf "%s\n" "\\." "\\copy game_rules FROM STDIN"; \
	  psql -U core_user -d core_studio -v ON_ERROR_STOP=1 -c "\\copy game_rules TO STDOUT"; \
	  printf "%s\n" "\\."; \
	} > $$f; psql -U core_user -d core_game -v ON_ERROR_STOP=1 --single-transaction -f $$f'

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
