package eventstore_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

func TestEventLoader_ReportsEntityTypeOfAggregate(t *testing.T) {
	_, registry := setup(t)

	loader := eventstore.NewEventLoader(registry, operator.NewID, operator.NewEventType)

	require.Equal(t, operatortest.ID(t, 1).EntityType(), loader.EntityType())
}

func TestEventLoader_LoadsAllEventsInVersionOrder(t *testing.T) {
	ctx, registry := setup(t)
	id, _, registered := register(t, ctx, registry, "operator@example.com", time.Now().UTC())
	store := operatorStore(registry)
	require.NoError(t, store.Save(ctx, registered))
	state, err := store.Load(ctx, id)
	require.NoError(t, err)
	linked, err := operator.Decide(state, &operator.LinkCommand{
		Issuer: operatortest.Issuer(t, "diecardomain-studio"), Subject: operatortest.Subject(t, "user-1"),
	}, time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, store.Save(ctx, linked))

	events, err := eventstore.NewEventLoader(registry, operator.NewID, operator.NewEventType).LoadEvents(ctx, id.Int64())

	require.NoError(t, err)
	require.Len(t, events, 2)
	require.IsType(t, &operator.RegisteredEvent{}, events[0].Event)
	require.Equal(t, int64(1), events[0].AggregateVersion)
	require.IsType(t, &operator.LinkedEvent{}, events[1].Event)
	require.Equal(t, int64(2), events[1].AggregateVersion)
}

func TestEventLoader_ReturnsNoEventsWhenAggregateHasNone(t *testing.T) {
	ctx, registry := setup(t)
	id, _, err := identifier.New(registry, operator.NewID).Issue(ctx)
	require.NoError(t, err)

	events, err := eventstore.NewEventLoader(registry, operator.NewID, operator.NewEventType).LoadEvents(ctx, id.Int64())

	require.NoError(t, err)
	require.Empty(t, events)
}

// 途中の 1 件でも復元できなければ、読めた分だけを返さない（欠けた状態で read model を書かないため）
func TestEventLoader_FailsWhenAnyEventCannotBeDecoded(t *testing.T) {
	ctx, registry := setup(t)
	id, _, events := register(t, ctx, registry, "operator@example.com", time.Now().UTC())
	require.NoError(t, operatorStore(registry).Save(ctx, events))
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	// operator が知らないイベント種別を、テストの中だけで足して入れる（rollback で消える）
	_, err = exec.ExecContext(ctx, `INSERT INTO domain_event_types (id, entity_type_id, name) VALUES (1999, 1000, 'eventstore_test_unknown')`)
	require.NoError(t, err)
	_, err = exec.ExecContext(ctx, `
		INSERT INTO domain_events (entity_type_id, entity_id, aggregate_version, event_type_id, event_type_version, payload, created_at)
		VALUES (1000, $1, 2, 1999, 1, '{}', now())`, id.Int64())
	require.NoError(t, err)
	// registry は起動時に読み込むため、足した種別を読み直す
	registry, err = domaindef.Load(ctx)
	require.NoError(t, err)

	loaded, err := eventstore.NewEventLoader(registry, operator.NewID, operator.NewEventType).LoadEvents(ctx, id.Int64())

	require.ErrorIs(t, err, domain.ErrUnknownEventType)
	require.Nil(t, loaded)
}
