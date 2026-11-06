-- プレイヤーの read model（行はプレイヤーの公開用の ID ごと）。値はイベントから導く。表示に使う値は metadata に持つ（cards と同じ決まり）。
-- 行を書く投影が 2 つある: players（player を購読）は行を作るだけで、player_starter_grants（starter_grant を購読）は metadata の
-- starter_grant の項目だけを書く。別の集約の投影どうしは動く順番が決まらないため、どちらが先に動いても同じ行になるよう、
-- それぞれ自分の項目だけを書き、行が無ければ作る
CREATE TABLE players (
  -- domain_entities への外部キーは張らない（ほかの read model と同じ）
  player_id UUID PRIMARY KEY,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE players IS 'プレイヤーの read model。players の投影が行を作り、player_starter_grants の投影が metadata の starter_grant だけを書く（どちらが先でも同じ行になる）';
COMMENT ON COLUMN players.player_id IS 'プレイヤーの公開用の ID（domain_entities.public_id）';
COMMENT ON COLUMN players.metadata IS '値（JSON）。項目と形は game の player_writer.go・player_starter_grant_writer.go（配布の状態 starter_grant） の json のタグが決める（項目を足しても表は変えない）';
