package eventstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

func setup(t *testing.T) (context.Context, *domaindef.Registry) {
	t.Helper()
	ctx := rdbtest.Tx(t)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return ctx, registry
}

// 偽の種別を足すテストは、テンプレートから複製した自分だけの DB で動かす（共有の core_test には一覧の種別の行があり、
// 重なると失敗するため。core#33）
func isolatedSetup(t *testing.T) (context.Context, *domaindef.Registry) {
	t.Helper()
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return ctx, registry
}

func operatorStore(registry *domaindef.Registry) domain.EventStore[operator.ID, operator.Event, operator.State] {
	return eventstore.New(registry, operator.NewID, operator.NewEventType, operator.InitialState, operator.Evolve)
}

func register(t *testing.T, ctx context.Context, registry *domaindef.Registry, email string, now time.Time) (operator.ID, uuid.UUID, []operator.Event) {
	t.Helper()
	id, publicID, err := identifier.New(registry, operator.NewID).Issue(ctx)
	require.NoError(t, err)
	events, err := operator.Decide(operator.InitialState(), &operator.RegisterCommand{
		ID: id, PublicID: publicID, Email: operatortest.Email(t, email),
	}, now)
	require.NoError(t, err)
	return id, publicID, events
}
