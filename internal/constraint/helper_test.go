package constraint_test

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"sync"
	"testing"

	"github.com/google/uuid"
)

var cardNameSuffixes sync.Map

// 開発用の DB に残っているカードの名前とぶつからないよう、テストごとに UUID を付ける（同じテストの中では同じ値）
func cardName(t *testing.T, base string) string {
	t.Helper()
	suffix, _ := cardNameSuffixes.LoadOrStore(t.Name(), uuid.NewString())
	return fmt.Sprintf("%s-%s", base, suffix)
}

// 開発用の DB の状態（ほかの行があるか）に左右されないことを確かめるため、テンプレートから複製した自分だけの DB で動かす
func isolatedSetup(t *testing.T) fixture {
	t.Helper()
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return fixture{ctx: ctx, registry: registry}
}
