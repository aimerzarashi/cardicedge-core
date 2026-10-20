-- スターターデッキを、用途から切り離したデッキのマスタ「プリセットデッキ」に名前を変える（core#26）。スターターにも NPC デッキにも
-- 使い、用途は用途の側の集約がプリセットデッキを参照する形で表す。0014・0015 は適用済みのため書き換えず、名前だけをここで変える。
-- id・行・イベントの payload は変えない（名前はドメインの宣言と起動時に照合するだけで、行は id で結びついているため）
UPDATE domain_entity_types SET name = 'preset_deck' WHERE id = 5000;
UPDATE domain_unique_key_types SET name = 'preset_deck.name' WHERE id = 4;
UPDATE domain_reference_types SET name = 'preset_deck.card' WHERE id = 1;
UPDATE event_subscribers SET name = 'preset_deck_list' WHERE id = 1001;

ALTER TABLE starter_deck_lists RENAME TO preset_deck_lists;
ALTER TABLE preset_deck_lists RENAME COLUMN starter_deck_id TO preset_deck_id;
ALTER TABLE preset_deck_lists RENAME CONSTRAINT starter_deck_lists_pkey TO preset_deck_lists_pkey;
ALTER INDEX idx_starter_deck_lists_sort_name_starter_deck_id RENAME TO idx_preset_deck_lists_sort_name_preset_deck_id;

COMMENT ON TABLE preset_deck_lists IS 'プリセットデッキの一覧と詳細の read model。削除されたプリセットデッキは行ごと消す';
COMMENT ON COLUMN preset_deck_lists.preset_deck_id IS 'プリセットデッキの公開 ID（domain_identifiers.public_id）';
