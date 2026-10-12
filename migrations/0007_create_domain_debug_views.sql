-- domain_identifiers・domain_events・domain_event_types を、種別の id を名前に展開して読むための確認用の VIEW（参照プロジェクトの api と同じ）。
-- テーブルには種別が数値の id で入っているため、SQLTools 等で中身を見るときに使う。アプリからは使わない

CREATE OR REPLACE VIEW domain_identifier_view AS
SELECT
    idn.id,
    idn.public_id,
    ent.name AS entity_type,
    idn.created_at
FROM domain_identifiers idn
JOIN domain_entity_types ent ON ent.id = idn.entity_type_id
ORDER BY idn.id;

COMMENT ON VIEW domain_identifier_view IS 'domain_identifiers を entity_type 名付きで参照する確認用の VIEW';

CREATE OR REPLACE VIEW domain_event_view AS
SELECT
    dev.id,
    dev.entity_id,
    ent.name AS entity_type,
    dev.aggregate_version,
    evt.name AS event_type,
    dev.event_type_version,
    dev.payload,
    dev.created_at
FROM domain_events dev
JOIN domain_entity_types ent ON ent.id = dev.entity_type_id
JOIN domain_event_types evt ON evt.id = dev.event_type_id
ORDER BY dev.id;

COMMENT ON VIEW domain_event_view IS 'domain_events を entity_type 名・event_type 名付きで参照する確認用の VIEW';

CREATE OR REPLACE VIEW domain_event_type_view AS
SELECT
    evt.id,
    ent.name AS entity_type,
    evt.name AS event_type
FROM domain_event_types evt
JOIN domain_entity_types ent ON ent.id = evt.entity_type_id
ORDER BY evt.id;

COMMENT ON VIEW domain_event_type_view IS 'domain_event_types を entity_type 名付きで参照する確認用の VIEW';
