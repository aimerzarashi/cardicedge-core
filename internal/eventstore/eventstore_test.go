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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/typeregistry"
)

func setup(t *testing.T) (context.Context, *typeregistry.Registry) {
	t.Helper()
	ctx := rdbtest.Tx(t)
	registry, err := typeregistry.Load(ctx)
	require.NoError(t, err)
	return ctx, registry
}

func operatorStore(registry *typeregistry.Registry) domain.EventStore[operator.ID, operator.Event, operator.State] {
	return eventstore.New(registry, operator.NewID, operator.NewEventType, operator.InitialState, operator.Evolve)
}

func register(t *testing.T, ctx context.Context, registry *typeregistry.Registry, email string, now time.Time) (operator.ID, uuid.UUID, []operator.Event) {
	t.Helper()
	id, publicID, err := identifier.New(registry, operator.NewID).Issue(ctx)
	require.NoError(t, err)
	events, err := operator.Decide(operator.InitialState(), &operator.RegisterCommand{
		ID: id, PublicID: publicID, Email: operatortest.Email(t, email),
	}, now)
	require.NoError(t, err)
	return id, publicID, events
}

func TestSaveAndLoad_RestoresStateFromEvents(t *testing.T) {
	ctx, registry := setup(t)
	occurredAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	id, publicID, events := register(t, ctx, registry, "operator@example.com", occurredAt)
	store := operatorStore(registry)

	require.NoError(t, store.Save(ctx, events))
	state, err := store.Load(ctx, id)

	require.NoError(t, err)
	require.Equal(t, operator.Operator{
		ID: id, Version: 1, PublicID: publicID,
		Email: operatortest.Email(t, "operator@example.com"),
		// created_at には DB の現在時刻ではなく発生時刻が残る
		RegisteredAt: occurredAt,
	}, normalize(state))
}

func TestSave_RejectsSameVersionTwice(t *testing.T) {
	ctx, registry := setup(t)
	_, _, events := register(t, ctx, registry, "operator@example.com", time.Now().UTC())
	store := operatorStore(registry)
	require.NoError(t, store.Save(ctx, events))

	err := store.Save(ctx, events)

	require.ErrorIs(t, err, domain.ErrVersionConflict)
}

func TestSave_StoresEventTypeVersion(t *testing.T) {
	ctx, registry := setup(t)
	id, _, events := register(t, ctx, registry, "operator@example.com", time.Now().UTC())
	require.NoError(t, operatorStore(registry).Save(ctx, events))

	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var version int
	require.NoError(t, exec.QueryRowContext(ctx,
		`SELECT event_type_version FROM domain_events WHERE entity_id = $1`, id.Int64()).Scan(&version))
	require.Equal(t, events[0].SchemaVersion(), version)
}

func TestLoad_ReturnsInitialStateWhenNoEvents(t *testing.T) {
	ctx, registry := setup(t)
	id, _, err := identifier.New(registry, operator.NewID).Issue(ctx)
	require.NoError(t, err)

	state, err := operatorStore(registry).Load(ctx, id)

	require.NoError(t, err)
	require.Equal(t, operator.InitialState(), state)
}

// entity_id は集約の種別をまたいで重なりうるため、Load はほかの種別のイベントを読まない
func TestLoad_IgnoresEventsOfOtherEntityTypes(t *testing.T) {
	ctx, registry := setup(t)
	id, _, events := register(t, ctx, registry, "operator@example.com", time.Now().UTC())
	require.NoError(t, operatorStore(registry).Save(ctx, events))
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	// テストの中だけの種別に、同じ entity_id・次の版のイベントを入れる（rollback で消える）
	_, err = exec.ExecContext(ctx, `INSERT INTO domain_entity_types (id, name) VALUES (1, 'eventstore_test')`)
	require.NoError(t, err)
	_, err = exec.ExecContext(ctx, `INSERT INTO domain_event_types (id, entity_type_id, name) VALUES (1, 1, 'RegisteredEvent')`)
	require.NoError(t, err)
	_, err = exec.ExecContext(ctx, `
		INSERT INTO domain_events (entity_type_id, entity_id, aggregate_version, event_type_id, event_type_version, payload, created_at)
		VALUES (1, $1, 2, 1, 1, '{}', now())`, id.Int64())
	require.NoError(t, err)

	state, err := operatorStore(registry).Load(ctx, id)

	require.NoError(t, err)
	require.Equal(t, int64(1), state.(operator.Operator).Version)
}

func TestIdentifier_IssuesDistinctIDsAndResolvesPublicID(t *testing.T) {
	ctx, registry := setup(t)
	issuer := identifier.New(registry, operator.NewID)
	id1, publicID1, err := issuer.Issue(ctx)
	require.NoError(t, err)
	id2, publicID2, err := issuer.Issue(ctx)
	require.NoError(t, err)

	require.NotEqual(t, id1, id2)
	require.NotEqual(t, publicID1, publicID2)
	resolved, err := issuer.Resolve(ctx, publicID2)
	require.NoError(t, err)
	require.Equal(t, id2, resolved)
}

func TestIdentifier_ResolveFailsForUnknownPublicID(t *testing.T) {
	ctx, registry := setup(t)

	_, err := identifier.New(registry, operator.NewID).Resolve(ctx, uuid.New())

	require.Error(t, err)
}

// DB から読んだ時刻は Location が UTC と異なりうるため、比較のために UTC にそろえる
func normalize(state operator.State) operator.State {
	if op, ok := state.(operator.Operator); ok {
		op.RegisteredAt = op.RegisteredAt.UTC()
		return op
	}
	return state
}
