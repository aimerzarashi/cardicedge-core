-- スターターデッキセットの read model（1 行だけ）。値はイベントから導く。写したカードは公開 ID と枚数だけ持ち、カード名・種類は
-- 書き写さない（プリセットデッキの read model と同じ）
CREATE TABLE starter_deck_sets (
  -- domain_entities への外部キーは張らない（ほかの read model と同じ）
  starter_deck_set_id UUID PRIMARY KEY,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE starter_deck_sets IS 'スターターデッキセットの read model（ゲーム全体で 1 行）。未設定のあいだは行が無い';
COMMENT ON COLUMN starter_deck_sets.starter_deck_set_id IS 'スターターデッキセットの公開 ID（domain_entities.public_id）';
COMMENT ON COLUMN starter_deck_sets.metadata IS '表示に使う値（イベントの値。更新の番号（集約の版）・日時、スターターデッキ（名前・写したカードの公開 ID と枚数・写した元のプリセットデッキの公開 ID と revision）の一覧）';
