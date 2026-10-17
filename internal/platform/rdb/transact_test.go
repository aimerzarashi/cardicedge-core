package rdb_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

// commit そのものを確かめるため、最後に rollback する rdbtest.Tx は使わず、複製した DB にテスト用の表を作る
func openTestDB(t *testing.T) (context.Context, *sql.DB) {
	t.Helper()
	db, _ := rdbtest.NewDatabase(t)
	_, err := db.ExecContext(t.Context(), `CREATE TABLE rdb_transact_test (name TEXT PRIMARY KEY)`)
	require.NoError(t, err)
	return rdb.WithDB(t.Context(), db), db
}

func insert(ctx context.Context, name string) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, `INSERT INTO rdb_transact_test (name) VALUES ($1)`, name)
	return err
}

func count(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM rdb_transact_test`).Scan(&n))
	return n
}

func TestTransact_CommitsWhenFnSucceeds(t *testing.T) {
	ctx, db := openTestDB(t)

	got, err := rdb.Transact(ctx, func(ctx context.Context) (string, error) {
		return "done", insert(ctx, "a")
	})

	require.NoError(t, err)
	require.Equal(t, "done", got)
	require.Equal(t, 1, count(t, db))
}

func TestTransact_RollsBackWhenFnFails(t *testing.T) {
	ctx, db := openTestDB(t)
	failure := errors.New("failure")

	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		require.NoError(t, insert(ctx, "a"))
		return struct{}{}, failure
	})

	require.ErrorIs(t, err, failure)
	require.Equal(t, 0, count(t, db))
}

func TestTransact_RollsBackAndRepanicsWhenFnPanics(t *testing.T) {
	ctx, db := openTestDB(t)

	require.PanicsWithValue(t, "boom", func() {
		_, _ = rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
			require.NoError(t, insert(ctx, "a"))
			panic("boom")
		})
	})
	require.Equal(t, 0, count(t, db))
}

func TestTransact_NestedJoinsOuterTransaction(t *testing.T) {
	ctx, db := openTestDB(t)
	failure := errors.New("outer failure")

	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
			return struct{}{}, insert(ctx, "inner")
		})
		require.NoError(t, err)
		return struct{}{}, failure
	})

	require.ErrorIs(t, err, failure)
	// 外側が rollback したら、内側の書き込みも残らない
	require.Equal(t, 0, count(t, db))
}

func TestTransact_FailsWithoutDBInContext(t *testing.T) {
	_, err := rdb.Transact(t.Context(), func(ctx context.Context) (struct{}, error) {
		return struct{}{}, nil
	})
	require.ErrorIs(t, err, rdb.ErrNoDBConnection)
}
