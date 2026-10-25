-- 数の上限（デッキは 1 人 10 個まで等）を、上限ごとの専用の表ではなく 1 つの表で持つ。何の上限か（上限の種別）と持ち主の単位は
-- ドメインが宣言し、インフラは「種別 × 持ち主ごとに使っている数を、上限未満のときだけ 1 増やす」仕組みだけを持つ
-- （一意性の domain_unique_keys・参照の domain_references と同じ考え方で、3 つ目の制約の表。core#39）
CREATE TABLE domain_quota_types (
  id SMALLINT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE
);

COMMENT ON TABLE domain_quota_types IS '上限の種別（何の数を数えるか）の名前と id の対応。行は起動時にコードの一覧（domaindef.Types）から入れ、ドメインの宣言と照合する';

-- 上限の値は表にも CHECK にも持たず、確保の文に渡す（上限を変えても DDL を変えず、種別ごとの表や CHECK も要らないため。
-- 下限の 0 以上だけが種別によらない決まりなので CHECK にする）。
-- owner_id を domain_entities への外部キーにするのは、domain_unique_keys.entity_id と同じく、存在しない持ち主の行を作らないため。
-- 持ち主の識別子を消したら数も意味を失うので CASCADE にする（参照のように消させない理由が無い）
CREATE TABLE domain_quotas (
  quota_type_id SMALLINT NOT NULL REFERENCES domain_quota_types(id),
  owner_id BIGINT NOT NULL REFERENCES domain_entities(id) ON DELETE CASCADE,
  used INTEGER NOT NULL CHECK (used >= 0),
  PRIMARY KEY (quota_type_id, owner_id)
);

COMMENT ON TABLE domain_quotas IS '数の上限の制約。種別 × 持ち主ごとに使っている数。作るコマンドと同じトランザクションで、上限未満のときだけ 1 増やす。イベントから作り直せない、書く側の整合性のための記録';
COMMENT ON COLUMN domain_quotas.quota_type_id IS 'domain_quota_types.id';
COMMENT ON COLUMN domain_quotas.owner_id IS '数える単位の持ち主の集約の domain_entities.id';
COMMENT ON COLUMN domain_quotas.used IS '使っている数。上限の値はここに持たず、確保の文に渡す。0 以上';

-- 種別の id を名前に展開して読むための確認用の VIEW（0006 と同じ）。アプリからは使わない
CREATE OR REPLACE VIEW domain_quota_view AS
SELECT
    q.owner_id,
    ent.name AS owner_entity_type,
    qt.name AS quota_type,
    q.used
FROM domain_quotas q
JOIN domain_quota_types qt ON qt.id = q.quota_type_id
JOIN domain_entities idn ON idn.id = q.owner_id
JOIN domain_entity_types ent ON ent.id = idn.entity_type_id
ORDER BY q.owner_id, qt.id;

COMMENT ON VIEW domain_quota_view IS 'domain_quotas を上限の種別名・持ち主の entity_type 名付きで参照する確認用の VIEW。アプリからは使わない';
