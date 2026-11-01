-- 重い SQL の集計（pg_stat_statements。core#83）。拡張は DB ごとに作る必要があり、core_game は create-databases.sql で作るので、その後（ファイル名の順）に
-- \connect で両方に作る。マイグレーションでは拡張に触らない決まり（環境の側の役目）のため、ここに置く
\connect core_studio
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
\connect core_game
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
