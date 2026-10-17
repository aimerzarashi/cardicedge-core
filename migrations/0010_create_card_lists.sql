-- カードの一覧の read model。Creature・Item・Spell のイベントから投影（Projection）が作る（参照の api の avatar_lists と同じ形）。
-- 検索・絞り込み・並べ替えに使う値は個別の列、表示に使う値は metadata に持つ（同じ項目が両方に要れば両方に持つ）。
-- 値はすべてイベントから導き、DB の現在時刻などには頼らない。read model には不変条件・制約を置かない
CREATE TABLE card_lists (
  -- domain_identifiers への外部キーは張らない。集約の削除は削除のイベントで表し、投影が行を消す（CASCADE に頼らない）
  card_id UUID PRIMARY KEY,
  -- 集約の種別の id（creature は 2000 等）で並べないのは、種別の id の大小に並び順の決まりを隠さないため
  card_type SMALLINT NOT NULL,
  -- 正規化は DB でせず、投影が card.NormalizeName で作る（重複の判定と同じ関数にそろえるため。core#16）。
  -- en_US.utf8 の照合順序で並べないのは、glibc の版で並び方が変わり、カーソルで続きを取る順と索引が崩れるため
  sort_name TEXT COLLATE "C" NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE card_lists IS 'カードの一覧の read model（Creature・Item・Spell）。削除されたカードは行ごと消す';
COMMENT ON COLUMN card_lists.card_id IS 'カードの公開 ID（creature・item・spell の domain_identifiers.public_id）';
COMMENT ON COLUMN card_lists.card_type IS 'カードの種類。並び順の値（1: Creature、2: Item、3: Spell）';
COMMENT ON COLUMN card_lists.sort_name IS '並べ替え・名前の部分一致に使う、正規化した名前（card.NormalizeName）';
COMMENT ON COLUMN card_lists.metadata IS '表示に使う値（イベントの値）';

-- 一覧の並び（種類 → 名前）と、カーソルで続きを取る位置。同じ順位の決め手を内部の ID にしないのは、
-- カーソルは中身を読めるため、内部の ID を外に出さないため
CREATE INDEX idx_card_lists_card_type_sort_name_card_id ON card_lists (card_type, sort_name, card_id);

-- id の 1〜999 は、テスト（internal/eventdriven）が登録・削除する購読者のために空けておく（参照の api と同じ）
INSERT INTO event_subscribers (id, kind, name) VALUES
  (1000, 'projection', 'card_list');
