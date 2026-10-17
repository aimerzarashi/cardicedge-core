package rdbtest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

func TestNewDatabase_CopiesMigratedTemplate(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)

	var count int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM domain_entity_types`).Scan(&count))
	require.Positive(t, count)
}

func TestNewDatabase_IsolatesDataFromOtherDatabases(t *testing.T) {
	first, _ := rdbtest.NewDatabase(t)
	second, _ := rdbtest.NewDatabase(t)
	_, err := first.ExecContext(t.Context(), `INSERT INTO domain_identifiers (entity_type_id) VALUES (1000)`)
	require.NoError(t, err)

	var count int
	require.NoError(t, second.QueryRowContext(t.Context(), `SELECT count(*) FROM domain_identifiers`).Scan(&count))
	require.Zero(t, count)
}
