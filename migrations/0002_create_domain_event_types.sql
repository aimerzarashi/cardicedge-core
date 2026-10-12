CREATE TABLE domain_event_types (
  id SMALLINT PRIMARY KEY,
  entity_type_id SMALLINT NOT NULL REFERENCES domain_entity_types(id),
  name TEXT NOT NULL,
  UNIQUE (entity_type_id, name)
);

COMMENT ON TABLE domain_event_types IS 'イベント種別マスタ';
COMMENT ON COLUMN domain_event_types.id IS '手動採番。entity_type と同じく 1〜999 はテスト・参照用、1000 以降は実在の集約';
COMMENT ON COLUMN domain_event_types.entity_type_id IS '所属する集約の種別。この列でどの集約のイベントかがわかるため、id に集約ごとの範囲は持たせない';
COMMENT ON COLUMN domain_event_types.name IS 'ドメイン層のイベント型名と一致させる（例: RegisteredEvent）';
