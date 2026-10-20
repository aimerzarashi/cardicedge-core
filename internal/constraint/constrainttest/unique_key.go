// テストで、制約の表（一意性 domain_unique_keys・参照 domain_references）を読む道具。テストが表の作りを知らずに済むようにする。
// ファイルは本体の internal/constraint と同じく、制約ごとに分ける
package constrainttest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

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
