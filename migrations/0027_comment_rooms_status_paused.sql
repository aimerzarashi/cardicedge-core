-- 対戦の後に終えた部屋の状態（paused。休止中）を足した。状態の値は列の型ではなくコードが決めるので、列のコメントだけを直す
COMMENT ON COLUMN rooms.status IS '状態（draft・preparing・playing・deciding_next・paused・closed）。閉じた部屋を一覧から除くのに使う';
