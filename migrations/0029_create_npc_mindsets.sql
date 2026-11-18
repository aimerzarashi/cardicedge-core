-- NPC の思考パターンの一覧と詳細の read model（npc_avatars と同じ形）。値はイベントから導く。
-- 行動の種類ごとの重みと採点基準ごとの重みは metadata に持つ。重みは一覧の絞り込み・並べ替えに使わないため個別の列にしない（0008 と同じ）。
-- 状態（準備中・公開中・使用不可）は一覧の絞り込みに使うため個別の列にし、表示用にも metadata に持つ（0010 と同じ）。
-- game には、公開中・使用不可のものを make publish-master が写す（core#188）
CREATE TABLE npc_mindsets (
  -- domain_entities への外部キーは張らない。削除は削除のイベントで表し、投影が行を消す
  npc_mindset_id UUID PRIMARY KEY,
  -- 正規化は投影が domain.NormalizeName（名前の比べ方はカード名と同じ）で作る。COLLATE "C" の理由は cards と同じ
  sort_name TEXT COLLATE "C" NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE npc_mindsets IS 'NPC の思考パターンの一覧と詳細の read model。削除された思考パターンは行ごと消す';
COMMENT ON COLUMN npc_mindsets.npc_mindset_id IS 'NPC の思考パターンの公開 ID（domain_entities.public_id）';
COMMENT ON COLUMN npc_mindsets.sort_name IS '並べ替え・名前の部分一致に使う、正規化した名前';
COMMENT ON COLUMN npc_mindsets.status IS 'NPC の思考パターンの状態（draft: 準備中、published: 公開中、retired: 使用不可）';
COMMENT ON COLUMN npc_mindsets.metadata IS '値（JSON）。項目と形は studio の readmodel.NPCMindsetMetadata の json のタグが決める（項目を足しても表は変えない）';

-- 一覧の並び（名前 → 公開 ID）と、カーソルで続きを取る位置。状態は 3 値で絞る効果が小さい（思考パターンの数も少ない）ため索引に入れない（0010 と同じ）
CREATE INDEX idx_npc_mindsets_sort_name_npc_mindset_id ON npc_mindsets (sort_name, npc_mindset_id);
