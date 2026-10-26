-- 集約どうしの参照（参照されている集約を消せない制約）を、参照ごとの専用のテーブルではなく 1 つのテーブルで持つ。
-- 何が何を参照するか（参照の種別）はドメインが宣言し、インフラは「参照の行を持ち、参照されている識別子を消させない」仕組みだけを持つ
-- （一意性の制約の domain_unique_keys と同じ考え方。参照の api の main は関係ごとに表を作っていた（operator_assigned_roles）が、
-- 参照を足すたびに表・SQL を足さないため、こちらにした。core#24）
CREATE TABLE domain_reference_types (
  id SMALLINT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE
);

COMMENT ON TABLE domain_reference_types IS '参照の種別（何が何を参照するか）の名前と id の対応。行は起動時にコードの一覧（domaindef.Types）から入れ、ドメインの宣言と照合する';

-- 参照する側を消したら参照も消え（CASCADE）、参照されている側は消せない（RESTRICT）。参照されている集約を消すコマンドは、
-- 消す前に参照を確かめて「使われている」と返す。確かめ忘れても、この制約が識別子の削除を止めるため、参照する側の投影などが壊れない
CREATE TABLE domain_references (
  reference_type_id SMALLINT NOT NULL REFERENCES domain_reference_types(id),
  referencing_id BIGINT NOT NULL REFERENCES domain_entities(id) ON DELETE CASCADE,
  referenced_id BIGINT NOT NULL REFERENCES domain_entities(id) ON DELETE RESTRICT,
  PRIMARY KEY (reference_type_id, referencing_id, referenced_id),
  -- 今の参照はどれも種類の違う集約どうしで、同じ ID になるのはコードの誤りのときだけのため（要る参照ができたら外す）
  CHECK (referencing_id <> referenced_id)
);

COMMENT ON TABLE domain_references IS '集約どうしの参照。参照する側のコマンドと同じトランザクションで足し引きする';
COMMENT ON COLUMN domain_references.reference_type_id IS 'domain_reference_types.id';
COMMENT ON COLUMN domain_references.referencing_id IS '参照する側の集約の domain_entities.id';
COMMENT ON COLUMN domain_references.referenced_id IS '参照される側の集約の domain_entities.id';

-- 「参照されているか」を引くため（主キーは参照する側から引く順のため使えない）
CREATE INDEX idx_domain_references_referenced_id ON domain_references (referenced_id);
