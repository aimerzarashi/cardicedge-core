-- core-db の初期化（データディレクトリが空のときに 1 回だけ動く）。core_studio は POSTGRES_DB で作るため、もう 1 つの
-- core_game をここで作る。最小の構成（サーバー 1 つに DB を分ける）を、開発・テストの環境でも同じ形にするため（core#33）
CREATE DATABASE core_game;
