-- 配布デッキの一覧と詳細の read model（preset_decks と同じ形）。値はイベントから導く。
-- 入れたカードは metadata に公開 ID と枚数だけ持ち、カード名・種類は書き写さない（preset_decks と同じ）。
-- 状態（準備中・公開中・使用不可）は、一覧の絞り込みと、make publish-master が game に写すデッキの選びに使うため個別の列にし、
-- 表示用にも metadata に持つ（cards と同じ。0008 の決まり）。
-- make publish-master は全行（状態つき）を game の DB に写し、game は公開中だけを読む。そのため core_studio と core_game で列を同じにする
CREATE TABLE grant_decks (
  -- domain_entities への外部キーは張らない。削除は削除のイベントで表し、投影が行を消す
  grant_deck_id UUID PRIMARY KEY,
  -- 正規化は投影が domain.NormalizeName（名前の比べ方はカード名と同じ）で作る。COLLATE "C" の理由は cards と同じ
  sort_name TEXT COLLATE "C" NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
  total_count INT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE grant_decks IS '配布デッキの一覧と詳細の read model。削除された配布デッキは行ごと消す。make publish-master は全行を game に写し、game は公開中だけを配る';
COMMENT ON COLUMN grant_decks.grant_deck_id IS '配布デッキの公開 ID（domain_entities.public_id）';
COMMENT ON COLUMN grant_decks.sort_name IS '並べ替え・名前の部分一致に使う、正規化した名前';
COMMENT ON COLUMN grant_decks.status IS '配布デッキの状態（draft: 準備中、published: 公開中、retired: 使用不可）。game は公開中だけを新しいプレイヤーに配る';
COMMENT ON COLUMN grant_decks.total_count IS 'カードの枚数の合計';
COMMENT ON COLUMN grant_decks.metadata IS '値（JSON）。項目と形は studio の readmodel.GrantDeckMetadata の json のタグが決める（項目を足しても表は変えない）';

-- 一覧の並び（名前 → 公開 ID）と、カーソルで続きを取る位置。状態は 3 値で絞る効果が小さい（デッキの数も少ない）ため索引に入れない（0008 と同じ）
CREATE INDEX idx_grant_decks_sort_name_grant_deck_id ON grant_decks (sort_name, grant_deck_id);
