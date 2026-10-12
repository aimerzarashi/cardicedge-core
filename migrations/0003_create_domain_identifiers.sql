CREATE TABLE domain_identifiers (
  id BIGSERIAL PRIMARY KEY,
  -- アプリでは作らず DB に任せる（INSERT の結果から読み取る）
  public_id UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  entity_type_id SMALLINT NOT NULL REFERENCES domain_entity_types(id),
  created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE domain_identifiers IS '全 Entity の ID の発行台帳（domain_events と対をなす）';
COMMENT ON COLUMN domain_identifiers.id IS '内部の ID（ドメイン層の ID 型）';
COMMENT ON COLUMN domain_identifiers.public_id IS '外部に公開する ID';
COMMENT ON COLUMN domain_identifiers.entity_type_id IS 'どの種別の ID か';

CREATE UNIQUE INDEX idx_domain_identifiers_type_public_id ON domain_identifiers(entity_type_id, public_id);
