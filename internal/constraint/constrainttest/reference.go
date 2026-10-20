package constrainttest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

const referencedQuery = `
	SELECT r.referenced_id FROM domain_references r JOIN domain_reference_types t ON t.id = r.reference_type_id
	WHERE t.name = $1 AND r.referencing_id = $2 ORDER BY r.referenced_id
`

// 参照する側が、その種別で参照している集約の ID（小さい順）
func Referenced(t testing.TB, ctx context.Context, referenceType domain.ReferenceType, referencingID int64) []int64 {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	rows, err := exec.QueryContext(ctx, referencedQuery, referenceType.String(), referencingID)
	require.NoError(t, err)
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}
