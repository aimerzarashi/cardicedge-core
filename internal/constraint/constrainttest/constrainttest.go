package constrainttest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// テストで、一意性の key からその持ち主（集約）を引く道具。テストが一意性のテーブルの作りを知らずに済むようにする

const ownerQuery = `
	SELECT u.entity_id FROM domain_unique_keys u JOIN domain_unique_key_types t ON t.id = u.unique_key_type_id
	WHERE t.name = $1 AND u.key = $2
`

// key を確保している集約の ID。確保されていなければテストを失敗させる
func Owner(t testing.TB, ctx context.Context, key domain.UniqueKey) int64 {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var entityID int64
	require.NoError(t, exec.QueryRowContext(ctx, ownerQuery, key.Type().String(), key.Value()).Scan(&entityID))
	return entityID
}
