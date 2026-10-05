package rdb_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

func TestOpen(t *testing.T) {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Skip("DB_DSN is not set. run via 'make test'")
	}

	db, err := rdb.Open(t.Context(), dsn)
	require.NoError(t, err)
	defer db.Close()

	var one int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT 1").Scan(&one))
	require.Equal(t, 1, one)
}

func TestOpen_FailsWhenUnreachable(t *testing.T) {
	_, err := rdb.Open(t.Context(), "host=127.0.0.1 port=1 user=x dbname=x sslmode=disable connect_timeout=1")
	require.Error(t, err)
}
