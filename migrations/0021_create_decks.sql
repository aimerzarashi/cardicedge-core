-- デッキの read model（行はデッキごと。値はイベントから導く）。表示に使う値は metadata に持つ（avatars と同じ決まり）。
-- プレイヤーが見る一覧（Player.decks）は player_id で絞り、作った順に並べるため、個別の列にする。
-- 入れたカードは metadata に公開 ID と枚数だけ持ち、カード名・種類は書き写さない（0010 と同じ）
CREATE TABLE decks (
  -- domain_entities への外部キーは張らない（ほかの read model と同じ）
  deck_id UUID PRIMARY KEY,
  player_id UUID NOT NULL,
  registered_at TIMESTAMPTZ NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE decks IS 'デッキの read model。作る操作（スターターの配布）しかない（名前・カードの変更、削除は未対応）ため、行は増えるだけ';
COMMENT ON COLUMN decks.deck_id IS 'デッキの公開用の ID（domain_entities.public_id）';
COMMENT ON COLUMN decks.player_id IS '持ち主のプレイヤーの公開用の ID（players.player_id と同じ値。外部キーは張らない）';
COMMENT ON COLUMN decks.registered_at IS '作った日時（イベントの発生時刻）。プレイヤーの一覧を作った順に並べるのに使う';
COMMENT ON COLUMN decks.metadata IS '値（JSON）。項目と形は game の readmodel.DeckMetadata の json のタグが決める（項目を足しても表は変えない）';

-- プレイヤーの一覧を作った順に取る（同じ日時でも順が決まるよう deck_id を末尾に置く）
CREATE INDEX idx_decks_player_id_registered_at_deck_id ON decks (player_id, registered_at, deck_id);
