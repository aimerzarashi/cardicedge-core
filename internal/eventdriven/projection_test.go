package eventdriven_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

type writes struct {
	upserted []countingReadModel
	deleted  []countingReadModel
}

func (w *writes) spec() eventdriven.ProjectionSpec[countingReadModel] {
	return eventdriven.ProjectionSpec[countingReadModel]{
		Name:          testProjectionName,
		Subscriptions: []domain.EntityType{operator.Types().EntityType},
		Upsert: func(_ context.Context, state countingReadModel) error {
			w.upserted = append(w.upserted, state)
			return nil
		},
		Delete: func(_ context.Context, state countingReadModel) error {
			w.deleted = append(w.deleted, state)
			return nil
		},
	}
}

// テストのトランザクションに購読者を足してから読み込み、operator のイベントの読み方を渡した Hub を作る
func newHub(t *testing.T) (context.Context, *eventdriven.Hub) {
	t.Helper()
	ctx := isolatedDatabase(t)
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	addTestSubscriber(t, ctx, exec)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	subscribers, err := eventdriven.LoadSubscribers(ctx)
	require.NoError(t, err)
	hub, err := eventdriven.NewHub(subscribers, registry, testOptions(1), eventstore.NewEventLoader(registry, operator.NewID, operator.NewEventType))
	require.NoError(t, err)
	return ctx, hub
}

func TestRegisterProjection_AcceptsDeclaredSubscriber(t *testing.T) {
	_, hub := newHub(t)

	err := eventdriven.RegisterProjection(hub, (&writes{}).spec())

	require.NoError(t, err)
}

func TestRegisterProjection_RejectsIncompleteSpec(t *testing.T) {
	tests := []struct {
		name   string
		modify func(spec *eventdriven.ProjectionSpec[countingReadModel])
	}{
		{name: "without_name", modify: func(spec *eventdriven.ProjectionSpec[countingReadModel]) { spec.Name = "" }},
		{name: "without_subscriptions", modify: func(spec *eventdriven.ProjectionSpec[countingReadModel]) { spec.Subscriptions = nil }},
		{name: "without_upsert", modify: func(spec *eventdriven.ProjectionSpec[countingReadModel]) { spec.Upsert = nil }},
		{name: "without_delete", modify: func(spec *eventdriven.ProjectionSpec[countingReadModel]) { spec.Delete = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, hub := newHub(t)
			spec := (&writes{}).spec()
			tt.modify(&spec)

			err := eventdriven.RegisterProjection(hub, spec)

			require.Error(t, err)
		})
	}
}

func TestRegisterProjection_RejectsSubscriberNotInTable(t *testing.T) {
	_, hub := newHub(t)
	spec := (&writes{}).spec()
	spec.Name = "not_in_event_subscribers"

	err := eventdriven.RegisterProjection(hub, spec)

	require.ErrorIs(t, err, eventdriven.ErrSubscriberNotFound)
}

func TestRegisterProjection_RejectsSameProjectionTwice(t *testing.T) {
	_, hub := newHub(t)
	require.NoError(t, eventdriven.RegisterProjection(hub, (&writes{}).spec()))

	err := eventdriven.RegisterProjection(hub, (&writes{}).spec())

	require.Error(t, err)
}

func TestRegisterProjection_RejectsInvalidSubscriptions(t *testing.T) {
	tests := []struct {
		name          string
		subscriptions []domain.EntityType
		wantErr       string
	}{
		{name: "unknown_entity_type", subscriptions: []domain.EntityType{domain.NewEntityType("eventdriven_test_unknown")}, wantErr: "entity type not found"},
		{name: "without_event_loader", subscriptions: []domain.EntityType{creature.Types().EntityType}, wantErr: "no event loader"},
		{name: "duplicate", subscriptions: []domain.EntityType{operator.Types().EntityType, operator.Types().EntityType}, wantErr: "duplicate subscription"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, hub := newHub(t)
			spec := (&writes{}).spec()
			spec.Subscriptions = tt.subscriptions

			err := eventdriven.RegisterProjection(hub, spec)

			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestNewHub_RejectsDuplicateEventLoader(t *testing.T) {
	ctx := isolatedDatabase(t)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	subscribers, err := eventdriven.LoadSubscribers(ctx)
	require.NoError(t, err)

	_, err = eventdriven.NewHub(subscribers, registry, testOptions(1),
		eventstore.NewEventLoader(registry, operator.NewID, operator.NewEventType),
		eventstore.NewEventLoader(registry, operator.NewID, operator.NewEventType),
	)

	require.Error(t, err)
}

func TestProjection_UpsertsStateEvolvedFromAllEvents(t *testing.T) {
	ctx, hub := newHub(t)
	w := &writes{}
	require.NoError(t, eventdriven.RegisterProjection(hub, w.spec()))

	err := eventdriven.Project(ctx, hub, testProjectionName, []eventstore.LoadedEvent{
		{Event: &operator.RegisteredEvent{}, AggregateVersion: 1},
		{Event: &operator.LinkedEvent{}, AggregateVersion: 2},
	})

	require.NoError(t, err)
	require.Equal(t, []countingReadModel{{Applied: 2}}, w.upserted)
	require.Empty(t, w.deleted)
}

func TestProjection_DeletesTerminalState(t *testing.T) {
	ctx, hub := newHub(t)
	w := &writes{}
	require.NoError(t, eventdriven.RegisterProjection(hub, w.spec()))

	err := eventdriven.Project(ctx, hub, testProjectionName, []eventstore.LoadedEvent{
		{Event: &operator.RegisteredEvent{}, AggregateVersion: 1},
		{Event: deletedEvent{}, AggregateVersion: 2},
	})

	require.NoError(t, err)
	require.Empty(t, w.upserted)
	require.Equal(t, []countingReadModel{{Applied: 1, Deleted: true}}, w.deleted)
}

func TestRegisterProjection_RejectsSubscriptionsDifferentFromTable(t *testing.T) {
	tests := []struct {
		name    string
		setup   string
		wantErr string
	}{
		{
			name:    "declared_but_not_in_table",
			setup:   `DELETE FROM event_subscriptions WHERE subscriber_id = 1`,
			wantErr: "is not in event_subscriptions",
		},
		{
			name:    "in_table_but_not_declared",
			setup:   `INSERT INTO event_subscriptions (subscriber_id, entity_type_id) VALUES (1, 2000)`,
			wantErr: "is not declared",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := isolatedDatabase(t)
			exec, err := rdb.GetExecutor(ctx)
			require.NoError(t, err)
			addTestSubscriber(t, ctx, exec)
			_, err = exec.ExecContext(ctx, tt.setup)
			require.NoError(t, err)
			registry, err := domaindef.Load(ctx)
			require.NoError(t, err)
			subscribers, err := eventdriven.LoadSubscribers(ctx)
			require.NoError(t, err)
			hub, err := eventdriven.NewHub(subscribers, registry, testOptions(1), eventstore.NewEventLoader(registry, operator.NewID, operator.NewEventType))
			require.NoError(t, err)

			err = eventdriven.RegisterProjection(hub, (&writes{}).spec())

			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// DB には、マイグレーションで入れた本物の投影（cards 等）の行があり、このテストではテスト用の投影しか登録しないため、
// 登録していない本物の投影を見つける。登録したテスト用の投影は出さない
func TestHub_VerifySubscribersReportsProjectionNotRegistered(t *testing.T) {
	_, hub := newHub(t)
	require.NoError(t, eventdriven.RegisterProjection(hub, (&writes{}).spec()))

	err := hub.VerifySubscribers()

	require.ErrorContains(t, err, "event_subscribers has a projection not registered in code: cards")
	require.NotContains(t, err.Error(), testProjectionName)
}
