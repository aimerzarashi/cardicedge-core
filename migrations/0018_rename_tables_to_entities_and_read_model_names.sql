-- 表の名前を見直す（core#27）。どれも名前だけで、id・行・列の値は変えない。
-- PostgreSQL は表の名前を変えても、付いている制約・連番・索引の名前を変えないため、ここで一緒に変える
-- （確認用の VIEW の定義は、表の名前の変更に自動で追従する）

-- ID を持つもの（集約の根と、将来公開する子の entity）の表。ID の発行台帳ではなく「entity が存在すること」を表すようになったため
-- （識別子を消すと entity を消したことになり、参照の外部キーもここを指す）。domain_entity_types と対にする
ALTER TABLE domain_identifiers RENAME TO domain_entities;
ALTER SEQUENCE domain_identifiers_id_seq RENAME TO domain_entities_id_seq;
ALTER TABLE domain_entities RENAME CONSTRAINT domain_identifiers_pkey TO domain_entities_pkey;
ALTER TABLE domain_entities RENAME CONSTRAINT domain_identifiers_public_id_key TO domain_entities_public_id_key;
ALTER TABLE domain_entities RENAME CONSTRAINT domain_identifiers_entity_type_id_fkey TO domain_entities_entity_type_id_fkey;
ALTER TABLE domain_entities RENAME CONSTRAINT domain_identifiers_id_not_null TO domain_entities_id_not_null;
ALTER TABLE domain_entities RENAME CONSTRAINT domain_identifiers_public_id_not_null TO domain_entities_public_id_not_null;
ALTER TABLE domain_entities RENAME CONSTRAINT domain_identifiers_entity_type_id_not_null TO domain_entities_entity_type_id_not_null;
ALTER TABLE domain_entities RENAME CONSTRAINT domain_identifiers_created_at_not_null TO domain_entities_created_at_not_null;
ALTER INDEX idx_domain_identifiers_type_public_id RENAME TO idx_domain_entities_type_public_id;
ALTER VIEW domain_identifier_view RENAME TO domain_entity_view;

COMMENT ON TABLE domain_entities IS 'ID を持つもの（集約の根と、公開する子の entity）。内部 ID と公開 ID の組。状態は持たない（状態は domain_events から作る）。行を消すと entity を消したことになる';
COMMENT ON COLUMN domain_entities.entity_type_id IS 'どの種別の entity か';
COMMENT ON VIEW domain_entity_view IS 'domain_entities を entity_type 名付きで参照する確認用の VIEW';

-- read model の表は、表すものの複数形にする（_lists は一覧のための意味だったが、詳細にも使い、1 行しかない表にも合わないため）。
-- 投影の名前も、表と同じにする（1 つの投影が 1 つの表を書くため）
ALTER TABLE card_lists RENAME TO cards;
ALTER TABLE cards RENAME CONSTRAINT card_lists_pkey TO cards_pkey;
ALTER TABLE cards RENAME CONSTRAINT card_lists_card_id_not_null TO cards_card_id_not_null;
ALTER TABLE cards RENAME CONSTRAINT card_lists_card_type_not_null TO cards_card_type_not_null;
ALTER TABLE cards RENAME CONSTRAINT card_lists_sort_name_not_null TO cards_sort_name_not_null;
ALTER TABLE cards RENAME CONSTRAINT card_lists_metadata_not_null TO cards_metadata_not_null;
ALTER INDEX idx_card_lists_card_type_sort_name_card_id RENAME TO idx_cards_card_type_sort_name_card_id;
COMMENT ON TABLE cards IS 'カード（Creature・Item・Spell）の一覧と、ほかの read model から引くカードの read model。削除されたカードは行ごと消す';

-- 0016 でプリセットデッキに名前を変えたとき、NOT NULL の制約の名前を変え忘れていたため、ここで一緒に直す
ALTER TABLE preset_deck_lists RENAME TO preset_decks;
ALTER TABLE preset_decks RENAME CONSTRAINT preset_deck_lists_pkey TO preset_decks_pkey;
ALTER TABLE preset_decks RENAME CONSTRAINT starter_deck_lists_starter_deck_id_not_null TO preset_decks_preset_deck_id_not_null;
ALTER TABLE preset_decks RENAME CONSTRAINT starter_deck_lists_sort_name_not_null TO preset_decks_sort_name_not_null;
ALTER TABLE preset_decks RENAME CONSTRAINT starter_deck_lists_total_count_not_null TO preset_decks_total_count_not_null;
ALTER TABLE preset_decks RENAME CONSTRAINT starter_deck_lists_metadata_not_null TO preset_decks_metadata_not_null;
ALTER INDEX idx_preset_deck_lists_sort_name_preset_deck_id RENAME TO idx_preset_decks_sort_name_preset_deck_id;
COMMENT ON TABLE preset_decks IS 'プリセットデッキの一覧と詳細の read model。削除されたプリセットデッキは行ごと消す';

UPDATE event_subscribers SET name = 'cards' WHERE id = 1000;
UPDATE event_subscribers SET name = 'preset_decks' WHERE id = 1001;
UPDATE event_subscribers SET name = 'starter_deck_sets' WHERE id = 1002;
