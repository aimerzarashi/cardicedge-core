-- core-db の初期化（データディレクトリが空のときに 1 回だけ動く）。core_studio は POSTGRES_DB で作るため、もう 1 つの
-- core_game をここで作る。開発・テストでは、core の 2 つの DB を core 専用のサーバーに同居させる（core#33。最小の構成（統合の
-- DB サーバー 1 台）はリリース確認の環境で確かめる）
CREATE DATABASE core_game;
