INSERT INTO domain_entity_types (id, name) VALUES (6000, 'starter_deck_set');
INSERT INTO domain_event_types (id, entity_type_id, name) VALUES
  (6000, 6000, 'DecksSetEvent');

-- スターターデッキセットはゲーム全体で 1 つ。key の値は決まった文字列で、2 つ目を作らせない（同時に初めて設定しても 1 つだけ通る）。
-- 1 つしかない集約の内部 ID を引くのにも使う
INSERT INTO domain_unique_key_types (id, name) VALUES
  (5, 'starter_deck_set.singleton');

-- スターターデッキに写したカードを、写している間は消せないようにする（プリセットデッキと同じ）
INSERT INTO domain_reference_types (id, name) VALUES
  (2, 'starter_deck_set.card');

-- スターターデッキセットの read model（1 行だけ）。値はイベントから導く。写したカードは公開 ID と枚数だけ持ち、カード名・種類は
-- 書き写さない（プリセットデッキの read model と同じ）
CREATE TABLE starter_deck_sets (
  -- domain_identifiers への外部キーは張らない（ほかの read model と同じ）
  starter_deck_set_id UUID PRIMARY KEY,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE starter_deck_sets IS 'スターターデッキセットの read model（ゲーム全体で 1 行）。未設定のあいだは行が無い';
COMMENT ON COLUMN starter_deck_sets.starter_deck_set_id IS 'スターターデッキセットの公開 ID（domain_identifiers.public_id）';
COMMENT ON COLUMN starter_deck_sets.metadata IS '表示に使う値（イベントの値。更新の番号（集約の版）・日時、スターターデッキ（名前・写したカードの公開 ID と枚数・写した元のプリセットデッキの公開 ID と revision）の一覧）';

INSERT INTO event_subscribers (id, kind, name) VALUES
  (1002, 'projection', 'starter_deck_set');

INSERT INTO event_subscriptions (subscriber_id, entity_type_id) VALUES
  (1002, 6000);
