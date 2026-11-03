-- プレイヤーの解放（unlock の集約。プレイヤーに 1 つ）の read model。表示に使う値は metadata に持つ（decks と同じ決まり）。
-- 解放したボードは metadata に公開 ID を解放した順で持ち、ボードの中身は書き写さない（読むときに写した boards から引く）。
-- プレイヤーが見る一覧（Player.boards）は player_id で絞るため、個別の列にする
CREATE TABLE unlocks (
  -- domain_entities への外部キーは張らない（ほかの read model と同じ）
  unlock_id UUID PRIMARY KEY,
  player_id UUID NOT NULL,
  registered_at TIMESTAMPTZ NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE unlocks IS 'プレイヤーの解放の read model。作る操作（スターターの配布）しかないため、行は増えるだけ';
COMMENT ON COLUMN unlocks.unlock_id IS '解放の公開用の ID（domain_entities.public_id）';
COMMENT ON COLUMN unlocks.player_id IS '持ち主のプレイヤーの公開用の ID（players.player_id と同じ値。外部キーは張らない）';
COMMENT ON COLUMN unlocks.registered_at IS '作った日時（イベントの発生時刻）';
COMMENT ON COLUMN unlocks.metadata IS '値（JSON）。項目と形は game の readmodel.UnlockMetadata の json のタグが決める（項目を足しても表は変えない）';

-- プレイヤーの解放を取る（プレイヤーに 1 つ。一意な key は集約の側が守るので、索引は引くためだけ）
CREATE INDEX idx_unlocks_player_id ON unlocks (player_id);
