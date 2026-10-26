-- プリセットデッキの一覧と詳細の read model（cards と同じ形）。値はイベントから導く。
-- 入れたカードは metadata に公開 ID と枚数だけ持ち、カード名・種類は書き写さない（カードの名前の変更のたびに直す必要が出ないため）。
-- 名前は読むときにデータローダーで cards から引く。カードを別の表に分けないのは、詳細は丸ごと読むだけで、カードで絞り込まないため
CREATE TABLE preset_decks (
  -- domain_entities への外部キーは張らない。削除は削除のイベントで表し、投影が行を消す
  preset_deck_id UUID PRIMARY KEY,
  -- 正規化は投影が card.NormalizeName（名前の比べ方はカード名と同じ）で作る。COLLATE "C" の理由は cards と同じ
  sort_name TEXT COLLATE "C" NOT NULL,
  total_count INT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE preset_decks IS 'プリセットデッキの一覧と詳細の read model。削除されたプリセットデッキは行ごと消す';
COMMENT ON COLUMN preset_decks.preset_deck_id IS 'プリセットデッキの公開 ID（domain_entities.public_id）';
COMMENT ON COLUMN preset_decks.sort_name IS '並べ替え・名前の部分一致に使う、正規化した名前';
COMMENT ON COLUMN preset_decks.total_count IS 'カードの枚数の合計';
COMMENT ON COLUMN preset_decks.metadata IS '表示に使う値（イベントの値。名前・更新の番号（集約の版）・日時、入れたカードの公開 ID と枚数の一覧（cards: [{card_id, count}]。枚数が 0 のカードは持たない））';

CREATE INDEX idx_preset_decks_sort_name_preset_deck_id ON preset_decks (sort_name, preset_deck_id);
