-- 部屋の read model（行は部屋ごと。値はイベントから導く）。表示に使う値は metadata に持つ（avatars・unlocks と同じ決まり）。
-- 自分の部屋の一覧（Query.myRooms）は持ち主で絞り、新しい順に並べて、閉じた部屋を除くため、持ち主・状態・作った日時は個別の列にする。
-- 席・対戦ルール・準備完了の人は公開 ID だけを持ち、アバター・デッキ・ボードの名前は書き写さない（読むときに avatars・decks・npc_avatars・npc_decks・boards から引く）
CREATE TABLE rooms (
  -- domain_entities への外部キーは張らない（ほかの read model と同じ）
  room_id UUID PRIMARY KEY,
  owner_player_id UUID NOT NULL,
  status TEXT NOT NULL,
  registered_at TIMESTAMPTZ NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE rooms IS '部屋の read model。部屋を消す操作は無い（閉じた部屋は状態が closed になる）ため、行は増えるだけ';
COMMENT ON COLUMN rooms.room_id IS '部屋の公開用の ID（domain_entities.public_id）';
COMMENT ON COLUMN rooms.owner_player_id IS '作ったプレイヤー（持ち主）の公開用の ID（players.player_id と同じ値。外部キーは張らない）。今は部屋に入れるのは作った人だけなので、自分の部屋の一覧はこれで絞る。ほかの人が参加できるようになるときに、入っている人の表を足す';
COMMENT ON COLUMN rooms.status IS '状態（draft・preparing・closed）。閉じた部屋を一覧から除くのに使う';
COMMENT ON COLUMN rooms.registered_at IS '作った日時（イベントの発生時刻）。自分の部屋の一覧を新しい順に並べるのに使う';
COMMENT ON COLUMN rooms.metadata IS '値（JSON）。項目と形は game の readmodel.RoomMetadata の json のタグが決める（項目を足しても表は変えない）';

-- 自分の部屋の一覧を新しい順に取る（同じ日時でも順が決まるよう room_id を末尾に置く）
CREATE INDEX idx_rooms_owner_player_id_registered_at_room_id ON rooms (owner_player_id, registered_at DESC, room_id DESC);
