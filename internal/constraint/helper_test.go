package constraint_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

type fixture struct {
	ctx      context.Context
	registry *domaindef.Registry
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := rdbtest.Tx(t)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return fixture{ctx: ctx, registry: registry}
}

var cardNameSuffixes sync.Map

// 同時に動くほかのテストのカードの名前とぶつからないよう、テストごとに UUID を付ける（同じテストの中では同じ値）
func cardName(t *testing.T, base string) string {
	t.Helper()
	suffix, _ := cardNameSuffixes.LoadOrStore(t.Name(), uuid.NewString())
	return fmt.Sprintf("%s-%s", base, suffix)
}

// 共有の core_test の状態（ほかのテストの行があるか）に左右されないことを確かめるため、テンプレートから複製した自分だけの DB で動かす
func isolatedSetup(t *testing.T) fixture {
	t.Helper()
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return fixture{ctx: ctx, registry: registry}
}

func (f fixture) issueCreatureID(t *testing.T) creature.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, creature.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func (f fixture) updateCardName(t *testing.T, id creature.ID, name string) error {
	t.Helper()
	return constraint.UpdateCreatureName(f.ctx, id, cardtest.Name(t, name))
}

func (f fixture) cardRef(t *testing.T, id creature.ID) card.Ref {
	t.Helper()
	return cardtest.Ref(t, creature.Types().EntityType, id.Int64())
}
