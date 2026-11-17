-- プレイヤーの所持品（collection の集約。プレイヤーに 1 つ）の read model。表示に使う値は metadata に持つ（decks と同じ決まり）。
-- 持っているカードは metadata に公開 ID と枚数だけ持ち、カード名・種類は書き写さない（読むときに写した cards から引く）。
-- プレイヤーが見る一覧（Player.collection）は player_id で絞るため、個別の列にする。
-- 種類・名前で並べる・種類で絞るのは、写した cards の列（card_type・sort_name）を使うので、この表には並べ替えの列を持たない
CREATE TABLE collections (
  -- domain_entities への外部キーは張らない（ほかの read model と同じ）
  collection_id UUID PRIMARY KEY,
  player_id UUID NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE collections IS 'プレイヤーの所持品の read model。カードを足すたびに同じ行を更新する。削除するイベントは無い';
COMMENT ON COLUMN collections.collection_id IS '所持品の公開用の ID（domain_entities.public_id）';
COMMENT ON COLUMN collections.player_id IS '持ち主のプレイヤーの公開用の ID（players.player_id と同じ値。外部キーは張らない）';
COMMENT ON COLUMN collections.metadata IS '値（JSON）。項目と形は game の readmodel.CollectionMetadata の json のタグが決める（項目を足しても表は変えない）';

-- プレイヤーの所持品を取る（プレイヤーに 1 つ。一意な key は集約の側が守るので、索引は引くためだけ）
CREATE INDEX idx_collections_player_id ON collections (player_id);
