-- 識別子を発行する対象（集約・集約でない Entity）の種別。id は手動で採番し、Go 側のレジストリが起動時に名前と照合する
CREATE TABLE domain_entity_types (
  id SMALLINT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE
);

COMMENT ON TABLE domain_entity_types IS 'エンティティ種別マスタ（集約・集約でない Entity を含む、識別子の発行対象の種類）';
COMMENT ON COLUMN domain_entity_types.id IS '手動採番。1〜999 はテスト・参照用、1000 以降は実在の集約';
COMMENT ON COLUMN domain_entity_types.name IS 'ドメイン層のパッケージ名と一致させる（例: operator）';
