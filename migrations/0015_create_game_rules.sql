-- ゲーム全体のルールの read model（1 行だけ）。値はイベントから導く。値の項目は metadata の rules に丸ごと持ち、項目を足しても表は変えない
-- （make publish-master が行をそのまま game に写し、game は metadata の rules をそのまま読む。core#43）
CREATE TABLE game_rules (
  -- domain_entities への外部キーは張らない（ほかの read model と同じ）
  game_rules_id UUID PRIMARY KEY,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

COMMENT ON TABLE game_rules IS 'ゲーム全体のルールの read model（ゲーム全体で 1 行）。studio が起動時に作るまでは行が無く、game には make publish-master が写す';
COMMENT ON COLUMN game_rules.game_rules_id IS 'ゲーム全体のルールの公開 ID（domain_entities.public_id）';
COMMENT ON COLUMN game_rules.metadata IS '値（イベントの値。更新の番号（集約の版）・日時、ルール rules（スターターデッキの枚数・プレイヤーが持てる数など）。項目はスネークケース）';
