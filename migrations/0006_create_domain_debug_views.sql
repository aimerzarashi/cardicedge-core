-- 種別の id を名前に展開して読むための確認用の VIEW（参照プロジェクトの api と同じ）。
-- テーブルには種別が数値の id で入っているため、SQLTools 等で中身を見るときに使う。アプリからは使わない

CREATE OR REPLACE VIEW domain_entity_view AS
SELECT
    idn.id,
    idn.public_id,
    ent.name AS entity_type,
    idn.created_at
FROM domain_entities idn
JOIN domain_entity_types ent ON ent.id = idn.entity_type_id
ORDER BY idn.id;

COMMENT ON VIEW domain_entity_view IS 'domain_entities を entity_type 名付きで参照する確認用の VIEW';

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

CREATE OR REPLACE VIEW domain_unique_key_view AS
SELECT
    uk.entity_id,
    ent.name AS entity_type,
    ukt.name AS unique_key_type,
    uk.key
FROM domain_unique_keys uk
JOIN domain_unique_key_types ukt ON ukt.id = uk.unique_key_type_id
JOIN domain_entities idn ON idn.id = uk.entity_id
JOIN domain_entity_types ent ON ent.id = idn.entity_type_id
ORDER BY uk.entity_id, ukt.id;

COMMENT ON VIEW domain_unique_key_view IS 'domain_unique_keys を key の種別名・entity_type 名付きで参照する確認用の VIEW。アプリからは使わない';

CREATE OR REPLACE VIEW domain_reference_view AS
SELECT
    rt.name AS reference_type,
    r.referencing_id,
    referencing_ent.name AS referencing_entity_type,
    r.referenced_id,
    referenced_ent.name AS referenced_entity_type
FROM domain_references r
JOIN domain_reference_types rt ON rt.id = r.reference_type_id
JOIN domain_entities referencing_idn ON referencing_idn.id = r.referencing_id
JOIN domain_entity_types referencing_ent ON referencing_ent.id = referencing_idn.entity_type_id
JOIN domain_entities referenced_idn ON referenced_idn.id = r.referenced_id
JOIN domain_entity_types referenced_ent ON referenced_ent.id = referenced_idn.entity_type_id
ORDER BY rt.id, r.referencing_id, r.referenced_id;

COMMENT ON VIEW domain_reference_view IS 'domain_references を参照の種別名・entity_type 名付きで参照する確認用の VIEW。アプリからは使わない';
