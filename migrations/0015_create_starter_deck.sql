INSERT INTO domain_entity_types (id, name) VALUES (5000, 'starter_deck');
INSERT INTO domain_event_types (id, entity_type_id, name) VALUES
  (5000, 5000, 'RegisteredEvent'),
  (5001, 5000, 'RenamedEvent'),
  (5002, 5000, 'CardCountsChangedEvent'),
  (5003, 5000, 'DeletedEvent');

INSERT INTO domain_unique_key_types (id, name) VALUES
  (4, 'starter_deck.name');

-- スターターデッキに入れたカードを、入れている間は消せないようにする
INSERT INTO domain_reference_types (id, name) VALUES
  (1, 'starter_deck.card');

-- スターターデッキの一覧と詳細の read model（card_lists と同じ形）。値はイベントから導く。
-- 入れたカードは metadata に公開 ID と枚数だけ持ち、カード名・種類は書き写さない（カードの名前の変更のたびに直す必要が出ないため）。
-- 名前は読むときにデータローダーで card_lists から引く。カードを別の表に分けないのは、詳細は丸ごと読むだけで、カードで絞り込まないため
CREATE TABLE starter_deck_lists (
  -- domain_identifiers への外部キーは張らない。削除は削除のイベントで表し、投影が行を消す
  starter_deck_id UUID PRIMARY KEY,
  -- 正規化は投影が card.NormalizeName（名前の比べ方はカード名と同じ）で作る。COLLATE "C" の理由は card_lists と同じ
  sort_name TEXT COLLATE "C" NOT NULL,
  total_count INT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE starter_deck_lists IS 'スターターデッキの一覧と詳細の read model。削除されたスターターデッキは行ごと消す';
COMMENT ON COLUMN starter_deck_lists.starter_deck_id IS 'スターターデッキの公開 ID（domain_identifiers.public_id）';
COMMENT ON COLUMN starter_deck_lists.sort_name IS '並べ替え・名前の部分一致に使う、正規化した名前';
COMMENT ON COLUMN starter_deck_lists.total_count IS 'カードの枚数の合計';
COMMENT ON COLUMN starter_deck_lists.metadata IS '表示に使う値（イベントの値。名前・更新の番号（集約の版）・日時、入れたカードの公開 ID と枚数の一覧（cards: [{card_id, count}]。枚数が 0 のカードは持たない））';

CREATE INDEX idx_starter_deck_lists_sort_name_starter_deck_id ON starter_deck_lists (sort_name, starter_deck_id);

INSERT INTO event_subscribers (id, kind, name) VALUES
  (1001, 'projection', 'starter_deck_list');

INSERT INTO event_subscriptions (subscriber_id, entity_type_id) VALUES
  (1001, 5000);
