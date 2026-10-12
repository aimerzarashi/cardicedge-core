-- オペレーターの一意性を守る制約テーブル。登録・紐づけのコマンドと同じトランザクションで確保する

CREATE TABLE operators (
  id BIGINT PRIMARY KEY REFERENCES domain_identifiers(id) ON DELETE CASCADE,
  email TEXT NOT NULL UNIQUE
);

COMMENT ON TABLE operators IS 'オペレーターの制約テーブル（1 オペレーター 1 行。メールアドレスの一意）';
COMMENT ON COLUMN operators.id IS 'domain_identifiers.id';
COMMENT ON COLUMN operators.email IS '登録したメールアドレス（初めてのログインで紐づけるときにだけ照合する）';

-- 紐づけを operators の列（NULL 可）にしないのは、INSERT ... ON CONFLICT DO NOTHING で確保するため
-- （UPDATE で一意制約に違反するとトランザクションが中断され、続けて先に紐づいた行を引けない）
-- 認証のたびに (iss, sub) からオペレーターを引くのにも使う。read model（projection）で引かないのは、反映が非同期で、
-- 紐づけた直後のリクエストでまだ見つからないことがあるため（紐づけのイベントと同じトランザクションで書くこの表なら、すぐに引ける）
CREATE TABLE operator_subjects (
  id BIGINT PRIMARY KEY REFERENCES operators(id) ON DELETE CASCADE,
  issuer TEXT NOT NULL,
  subject TEXT NOT NULL,
  UNIQUE (issuer, subject)
);

COMMENT ON TABLE operator_subjects IS 'オペレーターと JWT の (iss, sub) の紐づけの制約テーブル（1 オペレーター 1 行）';
COMMENT ON COLUMN operator_subjects.id IS 'operators.id';
COMMENT ON COLUMN operator_subjects.issuer IS 'JWT の iss（認証の発行元。cms の better-auth）';
COMMENT ON COLUMN operator_subjects.subject IS 'JWT の sub（発行元でのユーザー ID）';
