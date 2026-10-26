-- ID を持つもの（集約の根と、将来公開する子の entity）の表。「entity が存在すること」を表す
-- （識別子を消すと entity を消したことになり、参照の外部キーもここを指す）。domain_entity_types と対にする
CREATE TABLE domain_entities (
  id BIGSERIAL PRIMARY KEY,
  -- アプリでは作らず DB に任せる（INSERT の結果から読み取る）
  public_id UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  entity_type_id SMALLINT NOT NULL REFERENCES domain_entity_types(id),
  created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE domain_entities IS 'ID を持つもの（集約の根と、公開する子の entity）。内部 ID と公開 ID の組。状態は持たない（状態は domain_events から作る）。行を消すと entity を消したことになる';
COMMENT ON COLUMN domain_entities.id IS '内部の ID（ドメイン層の ID 型）';
COMMENT ON COLUMN domain_entities.public_id IS '外部に公開する ID';
COMMENT ON COLUMN domain_entities.entity_type_id IS 'どの種別の entity か';

CREATE UNIQUE INDEX idx_domain_entities_type_public_id ON domain_entities(entity_type_id, public_id);
