CREATE TABLE domain_events (
  id BIGSERIAL PRIMARY KEY,
  entity_type_id SMALLINT NOT NULL REFERENCES domain_entity_types(id),
  -- domain_identifiers への外部キーは張らない。ID を削除してもイベントは履歴として残し、
  -- 非同期に動く Projection・Policy が読み切る前に CASCADE で消えないようにするため
  entity_id BIGINT NOT NULL,
  aggregate_version BIGINT NOT NULL,
  event_type_id SMALLINT NOT NULL REFERENCES domain_event_types(id),
  event_type_version INT NOT NULL,
  payload JSONB NOT NULL,
  -- DB の DEFAULT ではなく、イベントの発生時刻（EventMeta.OccurredAt）を入れる
  created_at TIMESTAMP WITH TIME ZONE NOT NULL,
  -- 楽観的排他（同じ集約に同じ版のイベントを 2 つ保存させない）
  UNIQUE (entity_type_id, entity_id, aggregate_version)
);

COMMENT ON TABLE domain_events IS '全集約のイベントストア';
COMMENT ON COLUMN domain_events.entity_id IS '集約の内部の ID（domain_identifiers.id）';
COMMENT ON COLUMN domain_events.aggregate_version IS '同じ集約の中で 1 から順に増える版';
COMMENT ON COLUMN domain_events.event_type_version IS 'イベントのスキーマの版';
COMMENT ON COLUMN domain_events.payload IS 'イベントの本体（EventMeta を除く）';
COMMENT ON COLUMN domain_events.created_at IS 'イベントの発生時刻';
-- 集約のイベントの読み込み（entity_type_id, entity_id, aggregate_version 順）は UNIQUE 制約のインデックスで足りる。
-- created_at のインデックスは作らない（順序は aggregate_version で足りる。設計メモ 4 節）。
-- 新しいイベントを知らせる NOTIFY のトリガーは、Projection・Policy の仕組みを作るときに足す
