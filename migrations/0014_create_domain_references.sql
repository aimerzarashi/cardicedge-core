-- 集約どうしの参照（参照されている集約を消せない制約）を、参照ごとの専用のテーブルではなく 1 つのテーブルで持つ。
-- 何が何を参照するか（参照の種別）はドメインが宣言し、インフラは「参照の行を持ち、参照されている識別子を消させない」仕組みだけを持つ
-- （一意性の制約の domain_unique_keys と同じ考え方。参照の api の main は関係ごとに表を作っていた（operator_assigned_roles）が、
-- 参照を足すたびに表・SQL を足さないため、こちらにした。core#24）
CREATE TABLE domain_reference_types (
  id SMALLINT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE
);

COMMENT ON TABLE domain_reference_types IS '参照の種別（何が何を参照するか）の名前と id の対応。ドメインが宣言する名前と、起動時に照合する';

-- 参照する側を消したら参照も消え（CASCADE）、参照されている側は消せない（RESTRICT）。参照されている集約を消すコマンドは、
-- 消す前に参照を確かめて「使われている」と返す。確かめ忘れても、この制約が識別子の削除を止めるため、参照する側の投影などが壊れない
CREATE TABLE domain_references (
  reference_type_id SMALLINT NOT NULL REFERENCES domain_reference_types(id),
  referencing_id BIGINT NOT NULL REFERENCES domain_identifiers(id) ON DELETE CASCADE,
  referenced_id BIGINT NOT NULL REFERENCES domain_identifiers(id) ON DELETE RESTRICT,
  PRIMARY KEY (reference_type_id, referencing_id, referenced_id),
  -- 今の参照はどれも種類の違う集約どうしで、同じ ID になるのはコードの誤りのときだけのため（要る参照ができたら外す）
  CHECK (referencing_id <> referenced_id)
);

COMMENT ON TABLE domain_references IS '集約どうしの参照。参照する側のコマンドと同じトランザクションで足し引きする';
COMMENT ON COLUMN domain_references.reference_type_id IS 'domain_reference_types.id';
COMMENT ON COLUMN domain_references.referencing_id IS '参照する側の集約の domain_identifiers.id';
COMMENT ON COLUMN domain_references.referenced_id IS '参照される側の集約の domain_identifiers.id';

-- 「参照されているか」を引くため（主キーは参照する側から引く順のため使えない）
CREATE INDEX idx_domain_references_referenced_id ON domain_references (referenced_id);

CREATE OR REPLACE VIEW domain_reference_view AS
SELECT
    rt.name AS reference_type,
    r.referencing_id,
    referencing_ent.name AS referencing_entity_type,
    r.referenced_id,
    referenced_ent.name AS referenced_entity_type
FROM domain_references r
JOIN domain_reference_types rt ON rt.id = r.reference_type_id
JOIN domain_identifiers referencing_idn ON referencing_idn.id = r.referencing_id
JOIN domain_entity_types referencing_ent ON referencing_ent.id = referencing_idn.entity_type_id
JOIN domain_identifiers referenced_idn ON referenced_idn.id = r.referenced_id
JOIN domain_entity_types referenced_ent ON referenced_ent.id = referenced_idn.entity_type_id
ORDER BY rt.id, r.referencing_id, r.referenced_id;

COMMENT ON VIEW domain_reference_view IS 'domain_references を参照の種別名・entity_type 名付きで参照する確認用の VIEW。アプリからは使わない';
