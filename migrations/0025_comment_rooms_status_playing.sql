-- 部屋に対戦中（playing）の状態を足した。状態の値は列の型ではなくコードが決めるので、表の作りは変えず、列のコメントだけを直す
COMMENT ON COLUMN rooms.status IS '状態（draft・preparing・playing・closed）。閉じた部屋を一覧から除くのに使う';
