package migration_test

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/migration"
)

// newTestDB はテストごとに専用のデータベースを作り、終了時に削除する（開発用の DB を汚さない）。
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Skip("DB_DSN is not set. run via 'make test'")
	}

	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { admin.Close() })

	name := fmt.Sprintf("migration_test_%d", time.Now().UnixNano())
	_, err = admin.Exec("CREATE DATABASE " + name)
	require.NoError(t, err)
	t.Cleanup(func() { admin.Exec("DROP DATABASE " + name + " WITH (FORCE)") })

	// key=value 形式の DSN は後に書いた値が優先される
	db, err := sql.Open("postgres", dsn+" dbname="+name)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists))
	return exists
}

func recordedVersions(t *testing.T, db *sql.DB) []int {
	t.Helper()
	rows, err := db.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	require.NoError(t, err)
	defer rows.Close()
	var versions []int
	for rows.Next() {
		var v int
		require.NoError(t, rows.Scan(&v))
		versions = append(versions, v)
	}
	return versions
}

func TestLoad_SortsAndIgnoresNonSQL(t *testing.T) {
	ms, err := migration.Load(fstest.MapFS{
		"0002_second.sql": {Data: []byte("SELECT 2")},
		"0001_first.sql":  {Data: []byte("SELECT 1")},
		"embed.go":        {Data: []byte("package migrations")},
		".gitkeep":        {Data: nil},
	})
	require.NoError(t, err)
	require.Equal(t, []migration.Migration{
		{Version: 1, Name: "first", SQL: "SELECT 1"},
		{Version: 2, Name: "second", SQL: "SELECT 2"},
	}, ms)
}

func TestLoad_RejectsInvalidName(t *testing.T) {
	for _, name := range []string{"1_short.sql", "0001-hyphen.sql", "0001_Upper.sql", "0001.sql"} {
		_, err := migration.Load(fstest.MapFS{name: {Data: []byte("SELECT 1")}})
		require.Error(t, err, name)
	}
}

func TestLoad_RejectsDuplicateVersion(t *testing.T) {
	_, err := migration.Load(fstest.MapFS{
		"0001_a.sql": {Data: []byte("SELECT 1")},
		"0001_b.sql": {Data: []byte("SELECT 1")},
	})
	require.ErrorContains(t, err, "duplicate migration version 0001")
}

func TestApply_AppliesInOrderAndRecords(t *testing.T) {
	db := newTestDB(t)
	ms := []migration.Migration{
		{Version: 1, Name: "create_a", SQL: "CREATE TABLE a (id INT)"},
		// 2つ目は1つ目に依存する（順序どおりに適用されること）
		{Version: 2, Name: "insert_a", SQL: "INSERT INTO a VALUES (1)"},
	}

	applied, err := migration.Apply(t.Context(), db, ms)
	require.NoError(t, err)
	require.Len(t, applied, 2)
	require.Equal(t, 1, applied[0].Version)
	require.Equal(t, "insert_a", applied[1].Name)

	var name string
	var executionMs int
	require.NoError(t, db.QueryRow(`SELECT name, execution_ms FROM schema_migrations WHERE version = 1`).Scan(&name, &executionMs))
	require.Equal(t, "create_a", name)
	require.GreaterOrEqual(t, executionMs, 0)
}

func TestApply_AppliesOnlyPending(t *testing.T) {
	db := newTestDB(t)
	m1 := migration.Migration{Version: 1, Name: "create_a", SQL: "CREATE TABLE a (id INT)"}
	m2 := migration.Migration{Version: 2, Name: "create_b", SQL: "CREATE TABLE b (id INT)"}

	_, err := migration.Apply(t.Context(), db, []migration.Migration{m1})
	require.NoError(t, err)

	// 2回目は未適用の m2 だけを適用する（m1 を再実行すると CREATE TABLE が失敗する）
	applied, err := migration.Apply(t.Context(), db, []migration.Migration{m1, m2})
	require.NoError(t, err)
	require.Equal(t, []migration.Applied{{Version: 2, Name: "create_b", ExecutionMs: applied[0].ExecutionMs}}, applied)

	// 何も無ければ何もしない
	applied, err = migration.Apply(t.Context(), db, []migration.Migration{m1, m2})
	require.NoError(t, err)
	require.Empty(t, applied)
	require.Equal(t, []int{1, 2}, recordedVersions(t, db))
}

func TestApply_RunsMultipleStatementsInOneFile(t *testing.T) {
	db := newTestDB(t)
	// 関数本体の ; を含む複数の文を1ファイルで実行できること
	ms := []migration.Migration{{Version: 1, Name: "multi", SQL: `
CREATE FUNCTION one() RETURNS int AS $$
BEGIN
  RETURN 1;
END;
$$ LANGUAGE plpgsql;
CREATE TABLE a (id INT DEFAULT one());
`}}

	_, err := migration.Apply(t.Context(), db, ms)
	require.NoError(t, err)
	require.True(t, tableExists(t, db, "a"))
}

func TestApply_RollsBackFailedMigration(t *testing.T) {
	db := newTestDB(t)
	ms := []migration.Migration{
		{Version: 1, Name: "create_a", SQL: "CREATE TABLE a (id INT)"},
		// 途中まで成功して失敗する（b の作成も戻ること）
		{Version: 2, Name: "broken", SQL: "CREATE TABLE b (id INT); SELECT no_such_column FROM a"},
		{Version: 3, Name: "create_c", SQL: "CREATE TABLE c (id INT)"},
	}

	applied, err := migration.Apply(t.Context(), db, ms)
	require.ErrorContains(t, err, "failed to apply 0002_broken")
	require.Len(t, applied, 1)

	require.True(t, tableExists(t, db, "a"))
	require.False(t, tableExists(t, db, "b"))
	require.False(t, tableExists(t, db, "c"))
	require.Equal(t, []int{1}, recordedVersions(t, db))
}

func TestApply_RejectsGap(t *testing.T) {
	db := newTestDB(t)
	m1 := migration.Migration{Version: 1, Name: "create_a", SQL: "CREATE TABLE a (id INT)"}
	m2 := migration.Migration{Version: 2, Name: "create_b", SQL: "CREATE TABLE b (id INT)"}
	m3 := migration.Migration{Version: 3, Name: "create_c", SQL: "CREATE TABLE c (id INT)"}

	// 先に 2 が適用された状態で、未適用の 1 が来る（ブランチの順序が入れ替わった等）
	_, err := migration.Apply(t.Context(), db, []migration.Migration{m2})
	require.NoError(t, err)

	_, err = migration.Apply(t.Context(), db, []migration.Migration{m1, m2, m3})
	require.ErrorContains(t, err, "0001_create_a is not applied but older than the latest applied version 0002")

	// 黙って飛ばさず、何も適用しない
	require.False(t, tableExists(t, db, "a"))
	require.False(t, tableExists(t, db, "c"))
	require.Equal(t, []int{2}, recordedVersions(t, db))
}

func TestApply_ConcurrentRunsApplyOnce(t *testing.T) {
	db := newTestDB(t)
	ms := []migration.Migration{
		// 時間のかかるマイグレーションで、並行実行が重なるようにする
		{Version: 1, Name: "slow", SQL: "SELECT pg_sleep(0.3); CREATE TABLE a (id INT)"},
	}

	const runs = 5
	var wg sync.WaitGroup
	errs := make([]error, runs)
	counts := make([]int, runs)
	for i := range runs {
		wg.Go(func() {
			applied, err := migration.Apply(t.Context(), db, ms)
			errs[i] = err
			counts[i] = len(applied)
		})
	}
	wg.Wait()

	total := 0
	for i := range runs {
		require.NoError(t, errs[i])
		total += counts[i]
	}
	require.Equal(t, 1, total)
	require.Equal(t, []int{1}, recordedVersions(t, db))
}
