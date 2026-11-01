-- ボードの一覧と詳細の read model（starter_decks と同じ形）。値はイベントから導く。
-- 状態（準備中・公開中・使用不可）は一覧の絞り込みに使うため個別の列にし、表示用にも metadata に持つ（0010 と同じ）。
-- マスとつながりは、一覧と詳細の表示にだけ使い、検索・絞り込みに使わないため metadata に持ち、列にしない。
CREATE TABLE boards (
  -- domain_entities への外部キーは張らない。削除は削除のイベントで表し、投影が行を消す
  board_id UUID PRIMARY KEY,
  -- 正規化は投影が domain.NormalizeName（名前の比べ方はカード名と同じ）で作る。COLLATE "C" の理由は cards と同じ
  sort_name TEXT COLLATE "C" NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'retired')),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE boards IS 'ボードの一覧と詳細の read model。削除されたボードは行ごと消す';
COMMENT ON COLUMN boards.board_id IS 'ボードの公開 ID（domain_entities.public_id）';
COMMENT ON COLUMN boards.sort_name IS '並べ替え・名前の部分一致に使う、正規化した名前';
COMMENT ON COLUMN boards.status IS 'ボードの状態（draft: 準備中、published: 公開中、retired: 使用不可）';
COMMENT ON COLUMN boards.metadata IS '表示に使う値（イベントの値。名前・状態・更新の番号（集約の版）・日時・幅と高さ（width・height）、マス（tiles: [{x, y, kind}]。y → x の順）、つながり（connections: [{a: {x, y}, b: {x, y}}]。a → b の順））';

-- 一覧の並び（名前 → 公開 ID）と、カーソルで続きを取る位置。状態は 3 値で絞る効果が小さい（ボードの数も少ない）ため索引に入れない（0010 と同じ）
CREATE INDEX idx_boards_sort_name_board_id ON boards (sort_name, board_id);
