-- アバターの read model（行はアバターごと。値はイベントから導く）。表示に使う値は metadata に持つ（cards と同じ決まり）。
-- プレイヤーが見る一覧（Player.avatars）は player_id で絞り、作った順に並べるため、個別の列にする
CREATE TABLE avatars (
  -- domain_entities への外部キーは張らない（ほかの read model と同じ）
  avatar_id UUID PRIMARY KEY,
  player_id UUID NOT NULL,
  registered_at TIMESTAMPTZ NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE avatars IS 'アバターの read model。作る操作しかない（名前の変更・削除は未対応）ため、行は増えるだけ';
COMMENT ON COLUMN avatars.avatar_id IS 'アバターの公開用の ID（domain_entities.public_id）';
COMMENT ON COLUMN avatars.player_id IS '持ち主のプレイヤーの公開用の ID（players.player_id と同じ値。外部キーは張らない）';
COMMENT ON COLUMN avatars.registered_at IS '作った日時（イベントの発生時刻）。プレイヤーの一覧を作った順に並べるのに使う';
COMMENT ON COLUMN avatars.metadata IS '値（JSON）。項目と形は game の readmodel.AvatarMetadata の json のタグが決める（項目を足しても表は変えない）';

-- プレイヤーの一覧を作った順に取る（同じ日時でも順が決まるよう avatar_id を末尾に置く）
CREATE INDEX idx_avatars_player_id_registered_at_avatar_id ON avatars (player_id, registered_at, avatar_id);
