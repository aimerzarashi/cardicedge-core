INSERT INTO domain_entity_types (id, name) VALUES (2000, 'creature');
INSERT INTO domain_event_types (id, entity_type_id, name) VALUES
  (2000, 2000, 'RegisteredEvent');

-- 一意性の制約を、制約ごとの専用のテーブルではなく 1 つのテーブルで持つ。何を一意にするか（key の種別と key の作り方）はドメインが宣言し、
-- インフラは「key の種別の中で key を一意に確保する」仕組みだけを持つ（制約を足すたびにテーブル・SQL を足さないため。
-- 1 つの表で全集約のイベントを持つ domain_events と同じ考え方）。210 万件での計測では、専用のテーブルと比べて
-- 引く・確保する速さの差は誤差の範囲だった（2026-10-12、core#11）
CREATE TABLE domain_unique_key_types (
  id SMALLINT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE
);

COMMENT ON TABLE domain_unique_key_types IS '一意な key の種別（何を一意にするか）の名前と id の対応。ドメインが宣言する名前と、起動時に照合する';

-- 種別を名前の文字列で持たないのは、行と 2 つのインデックスのすべてに繰り返し入り、容量が増えるため（計測で約 1.4 倍）
INSERT INTO domain_unique_key_types (id, name) VALUES
  (1, 'operator.email'),
  (2, 'operator.subject'),
  (3, 'card.name');

-- 主キーを (unique_key_type_id, entity_id) にするのは、1 つの集約が 1 つの種別で持つ key を 1 つにし、変更のときに同じ行を更新するため
CREATE TABLE domain_unique_keys (
  unique_key_type_id SMALLINT NOT NULL REFERENCES domain_unique_key_types(id),
  key TEXT NOT NULL,
  entity_id BIGINT NOT NULL REFERENCES domain_identifiers(id) ON DELETE CASCADE,
  PRIMARY KEY (unique_key_type_id, entity_id),
  UNIQUE (unique_key_type_id, key)
);

COMMENT ON TABLE domain_unique_keys IS '一意性の制約。key の種別の中で key が一意。登録・変更のコマンドと同じトランザクションで確保する';
COMMENT ON COLUMN domain_unique_keys.unique_key_type_id IS 'domain_unique_key_types.id';
COMMENT ON COLUMN domain_unique_keys.key IS 'ドメインが作る一意にしたい値（正規化済み。複数の値は曖昧にならない形でつなぐ）';
COMMENT ON COLUMN domain_unique_keys.entity_id IS 'key を確保している集約の domain_identifiers.id';

-- 0006 の制約ごとのテーブルから移し、消す。(iss, sub) の key は、iss の文字数を前に付けてつなぐ（operator.SubjectKey と同じ形）
INSERT INTO domain_unique_keys (unique_key_type_id, key, entity_id)
SELECT 1, email, id FROM operators;
INSERT INTO domain_unique_keys (unique_key_type_id, key, entity_id)
SELECT 2, char_length(issuer) || ':' || issuer || subject, id FROM operator_subjects;
DROP TABLE operator_subjects;
DROP TABLE operators;

CREATE OR REPLACE VIEW domain_unique_key_view AS
SELECT
    uk.entity_id,
    ent.name AS entity_type,
    ukt.name AS unique_key_type,
    uk.key
FROM domain_unique_keys uk
JOIN domain_unique_key_types ukt ON ukt.id = uk.unique_key_type_id
JOIN domain_identifiers idn ON idn.id = uk.entity_id
JOIN domain_entity_types ent ON ent.id = idn.entity_type_id
ORDER BY uk.entity_id, ukt.id;

COMMENT ON VIEW domain_unique_key_view IS 'domain_unique_keys を key の種別名・entity_type 名付きで参照する確認用の VIEW。アプリからは使わない';
