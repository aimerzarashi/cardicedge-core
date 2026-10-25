-- カードの状態（準備中・公開中・使用不可）。一覧の絞り込みと、make publish-master が写すカードの選び（準備中は写さない）に使うため、
-- metadata ではなく個別の列にする（表示用の status は metadata にも持つ。0008 の決まり）。
-- 既定を準備中にするのは、集約の登録したときの状態と同じにするため。
-- 索引は足さない: 一覧の並びは (card_type, sort_name, card_id) の索引で取り、状態は 3 値で絞る効果が小さい（カードの数も少ない）ため、
-- 状態を先頭に入れると並びに使えなくなり、後ろに入れても読む行は減らない
ALTER TABLE cards ADD COLUMN status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired'));

COMMENT ON COLUMN cards.status IS 'カードの状態（draft: 準備中、published: 公開中、retired: 使用不可）。make publish-master は準備中を game に写さない';
