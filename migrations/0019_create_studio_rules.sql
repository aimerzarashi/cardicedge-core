-- studio でマスタを作るときの決まりの read model（1 行だけ）。game_rules（0015）と同じ形。値の項目は metadata の rules に丸ごと持ち、
-- 項目を足しても表は変えない。game には写さない（game では使わない）が、マイグレーションは両方の DB に同じものを流す決まりなので、
-- core_game にも空の表ができる（core#52）
CREATE TABLE studio_rules (
  -- domain_entities への外部キーは張らない（ほかの read model と同じ）
  studio_rules_id UUID PRIMARY KEY,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE studio_rules IS 'studio でマスタを作るときの決まりの read model（全体で 1 行）。studio が起動時に作るまでは行が無い。game には写さない';
COMMENT ON COLUMN studio_rules.studio_rules_id IS 'studio_rules の公開 ID（domain_entities.public_id）';
COMMENT ON COLUMN studio_rules.metadata IS '値（イベントの値。更新の番号（集約の版）・日時、ルール rules（ボードの幅・高さの範囲など）。項目はスネークケース）';
