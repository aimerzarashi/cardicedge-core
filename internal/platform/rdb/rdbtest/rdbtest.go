package rdbtest

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// DB_DSN の DB にはマイグレーションを適用済みであること
func Open(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Skip("DB_DSN is not set. run via 'make test'")
	}
	db, err := rdb.Open(t.Context(), dsn)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// テストの終わりに rollback するのは、開発用の DB でテストしても、テストのデータを残さないため
func Tx(t *testing.T) context.Context {
	t.Helper()
	db := Open(t)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { tx.Rollback() })
	return rdb.WithTx(rdb.WithDB(t.Context(), db), tx)
}
