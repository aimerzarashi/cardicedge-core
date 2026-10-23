-- 識別子を発行する対象（集約・集約でない Entity）の種別と、イベントの種別。
-- 行はマイグレーションでは入れず、起動時にコードの一覧（internal/domaindef の Types）から入れる（core#33）。
-- core_studio・core_game の 2 つの DB に同じマイグレーションを流し、種別は全サービスで共通の id で両方に入れるため
-- （マイグレーションで入れると、DB ごとに二重に管理するか、番号の調整が要る）。id は自動で振らない（DB ごと・起動の順番で変わりうるため）
CREATE TABLE domain_entity_types (
  id SMALLINT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE
);

COMMENT ON TABLE domain_entity_types IS 'エンティティ種別マスタ（集約・集約でない Entity を含む、識別子の発行対象の種類）';
COMMENT ON COLUMN domain_entity_types.id IS 'コードの一覧（domaindef.Types）で決める。集約は 1000 から 100 刻み';
COMMENT ON COLUMN domain_entity_types.name IS 'ドメイン層のパッケージ名と一致させる（例: operator）';

CREATE TABLE domain_event_types (
  id SMALLINT PRIMARY KEY,
  entity_type_id SMALLINT NOT NULL REFERENCES domain_entity_types(id),
  name TEXT NOT NULL,
  UNIQUE (entity_type_id, name)
);

COMMENT ON TABLE domain_event_types IS 'イベント種別マスタ';
COMMENT ON COLUMN domain_event_types.id IS 'コードの一覧（domaindef.Types）で決める。所属する集約の id ＋ 1 から順に';
COMMENT ON COLUMN domain_event_types.entity_type_id IS '所属する集約の種別';
COMMENT ON COLUMN domain_event_types.name IS 'ドメイン層のイベント型名と一致させる（例: RegisteredEvent）';
