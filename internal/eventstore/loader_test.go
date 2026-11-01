package eventstore_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

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
	ctx, registry := isolatedSetup(t)
	id, _, events := register(t, ctx, registry, "operator@example.com", time.Now().UTC())
	require.NoError(t, operatorStore(registry).Save(ctx, events))
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	// operator が知らないイベント種別を、テストの中だけで足して入れる
	_, err = exec.ExecContext(ctx, `INSERT INTO domain_event_types (id, entity_type_id, name) VALUES (1999, 1000, 'eventstore_test_unknown')`)
	require.NoError(t, err)
	_, err = exec.ExecContext(ctx, `
		INSERT INTO domain_events (entity_type_id, entity_id, aggregate_version, event_type_id, event_type_version, payload, occurred_at)
		VALUES (1000, $1, 2, 1999, 1, '{}', now())`, id.Int64())
	require.NoError(t, err)
	// registry は起動時に読み込むため、足した種別を読み直す
	registry, err = domaindef.Load(ctx)
	require.NoError(t, err)

	loaded, err := eventstore.NewEventLoader(registry, operator.NewID, operator.NewEventType).LoadEvents(ctx, id.Int64())

	require.ErrorIs(t, err, domain.ErrUnknownEventType)
	require.Nil(t, loaded)
}

func tracedContext(ctx context.Context) (context.Context, trace.SpanContext) {
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36},
		SpanID:     trace.SpanID{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7},
		TraceFlags: trace.FlagsSampled,
	})
	return trace.ContextWithSpanContext(ctx, sc), sc
}

func storedTraceparent(t *testing.T, ctx context.Context, id operator.ID) sql.NullString {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var traceparent sql.NullString
	require.NoError(t, exec.QueryRowContext(ctx, `SELECT traceparent FROM domain_events WHERE entity_id = $1`, id.Int64()).Scan(&traceparent))
	return traceparent
}

func TestSave_StoresTraceparentOfContext(t *testing.T) {
	ctx, registry := setup(t)
	id, _, events := register(t, ctx, registry, "operator@example.com", time.Now().UTC())
	traced, _ := tracedContext(ctx)

	require.NoError(t, operatorStore(registry).Save(traced, events))

	require.Equal(t, sql.NullString{String: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", Valid: true}, storedTraceparent(t, ctx, id))
}

func TestSave_StoresNullTraceparentWithoutTrace(t *testing.T) {
	ctx, registry := setup(t)
	id, _, events := register(t, ctx, registry, "operator@example.com", time.Now().UTC())

	require.NoError(t, operatorStore(registry).Save(ctx, events))

	require.False(t, storedTraceparent(t, ctx, id).Valid)
}

func TestEventLoader_ReturnsTraceOfSavedEvent(t *testing.T) {
	ctx, registry := setup(t)
	id, _, events := register(t, ctx, registry, "operator@example.com", time.Now().UTC())
	traced, want := tracedContext(ctx)
	require.NoError(t, operatorStore(registry).Save(traced, events))

	loaded, err := eventstore.NewEventLoader(registry, operator.NewID, operator.NewEventType).LoadEvents(ctx, id.Int64())

	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.Equal(t, want.TraceID(), loaded[0].SpanContext.TraceID())
	require.Equal(t, want.SpanID(), loaded[0].SpanContext.SpanID())
	require.True(t, loaded[0].SpanContext.IsRemote())
}

func TestEventLoader_ReturnsNoTraceWhenSavedWithoutTrace(t *testing.T) {
	ctx, registry := setup(t)
	id, _, events := register(t, ctx, registry, "operator@example.com", time.Now().UTC())
	require.NoError(t, operatorStore(registry).Save(ctx, events))

	loaded, err := eventstore.NewEventLoader(registry, operator.NewID, operator.NewEventType).LoadEvents(ctx, id.Int64())

	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.False(t, loaded[0].SpanContext.IsValid())
}

// 読めない traceparent があっても、イベントの復元は失敗させない（処理のログに trace_id が付かないだけにする）
func TestEventLoader_TreatsMalformedTraceparentAsNoTrace(t *testing.T) {
	ctx, registry := setup(t)
	id, _, events := register(t, ctx, registry, "operator@example.com", time.Now().UTC())
	require.NoError(t, operatorStore(registry).Save(ctx, events))
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	_, err = exec.ExecContext(ctx, `UPDATE domain_events SET traceparent = 'broken' WHERE entity_id = $1`, id.Int64())
	require.NoError(t, err)

	loaded, err := eventstore.NewEventLoader(registry, operator.NewID, operator.NewEventType).LoadEvents(ctx, id.Int64())

	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.False(t, loaded[0].SpanContext.IsValid())
}
