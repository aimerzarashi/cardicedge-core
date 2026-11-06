-- 対戦の ID から、その対戦を始めた部屋を引く（対戦の集約は部屋を持たないため、部屋の側の「始めた対戦の一覧」から逆に引く）。
-- 一覧は部屋の metadata の matches（公開 ID の配列）にあるので、列は足さず、その配列に索引を張って包含（@>）で引く。
-- 対戦の詳細は画面が繰り返し読むため、表の全体を舐めない索引にする
CREATE INDEX idx_rooms_matches ON rooms USING GIN ((metadata -> 'matches') jsonb_path_ops);

-- 対戦が終わった後の状態（deciding_next）を足した。状態の値は列の型ではなくコードが決めるので、列のコメントだけを直す
COMMENT ON COLUMN rooms.status IS '状態（draft・preparing・playing・deciding_next・closed）。閉じた部屋を一覧から除くのに使う';
