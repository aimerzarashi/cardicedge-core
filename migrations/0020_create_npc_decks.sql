-- NPC のデッキの一覧と詳細の read model（grant_decks と同じ形）。値はイベントから導く。
-- 入れたカードは metadata に公開 ID と枚数だけ持ち、カード名・種類は書き写さない（0010 と同じ）。
-- 状態（準備中・公開中・使用不可）は一覧の絞り込みに使うため個別の列にし、表示用にも metadata に持つ（0010 と同じ）。
-- game には、NPC のアバターに割り当てたものだけを make publish-master が写す（core#63）
CREATE TABLE npc_decks (
  -- domain_entities への外部キーは張らない。削除は削除のイベントで表し、投影が行を消す
  npc_deck_id UUID PRIMARY KEY,
  -- 正規化は投影が domain.NormalizeName（名前の比べ方はカード名と同じ）で作る。COLLATE "C" の理由は cards と同じ
  sort_name TEXT COLLATE "C" NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
  total_count INT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE npc_decks IS 'NPC のデッキの一覧と詳細の read model。削除された NPC のデッキは行ごと消す';
COMMENT ON COLUMN npc_decks.npc_deck_id IS 'NPC のデッキの公開 ID（domain_entities.public_id）';
COMMENT ON COLUMN npc_decks.sort_name IS '並べ替え・名前の部分一致に使う、正規化した名前';
COMMENT ON COLUMN npc_decks.status IS 'NPC のデッキの状態（draft: 準備中、published: 公開中、retired: 使用不可）';
COMMENT ON COLUMN npc_decks.total_count IS 'カードの枚数の合計';
COMMENT ON COLUMN npc_decks.metadata IS '値（JSON）。項目と形は studio の readmodel.NPCDeckMetadata の json のタグが決める（項目を足しても表は変えない）';

-- 一覧の並び（名前 → 公開 ID）と、カーソルで続きを取る位置。状態は 3 値で絞る効果が小さい（デッキの数も少ない）ため索引に入れない（0010 と同じ）
CREATE INDEX idx_npc_decks_sort_name_npc_deck_id ON npc_decks (sort_name, npc_deck_id);
