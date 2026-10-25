package creature_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature/creaturetest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
)

var (
	registeredAt = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
)

type fixture struct {
	id           creature.ID
	name         card.Name
	registeredBy operator.ID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{
		id:           creaturetest.ID(t, 20),
		name:         cardtest.Name(t, "ドラゴン"),
		registeredBy: operatortest.ID(t, 10),
	}
}

func (f fixture) registerCommand() *creature.RegisterCommand {
	return &creature.RegisterCommand{ID: f.id, PublicID: publicID, Name: f.name, RegisteredBy: f.registeredBy}
}

func (f fixture) register(t *testing.T) (creature.State, creature.Event) {
	t.Helper()
	events, err := creature.Decide(creature.InitialState(), f.registerCommand(), registeredAt)
	require.NoError(t, err)
	require.Len(t, events, 1)
	state, err := creature.Evolve(creature.InitialState(), events[0])
	require.NoError(t, err)
	return state, events[0]
}

func TestDecide_RegisterEmitsRegisteredEvent(t *testing.T) {
	f := newFixture(t)

	_, event := f.register(t)

	require.Equal(t, &creature.RegisteredEvent{
		EventMeta:    creature.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:     publicID,
		Name:         f.name,
		RegisteredBy: f.registeredBy,
	}, event)
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	_, err := creature.Decide(registered, f.registerCommand(), registeredAt)

	require.ErrorIs(t, err, creature.ErrInvalidState)
}

func TestEvolve_RegisteredEventMakesCreature(t *testing.T) {
	f := newFixture(t)

	registered, _ := f.register(t)

	require.Equal(t, creature.Creature{
		ID: f.id, Version: 1, PublicID: publicID, Name: f.name, Status: card.StatusDraft, RegisteredBy: f.registeredBy, RegisteredAt: registeredAt,
	}, registered)
}

func TestEvolve_RejectsRegisteredEventWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)
	registered, event := f.register(t)

	_, err := creature.Evolve(registered, event)

	require.ErrorIs(t, err, creature.ErrInvalidState)
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	f := newFixture(t)
	_, registered := f.register(t)
	state, _ := f.register(t)
	_, published := f.publish(t, state)
	_, retired := f.retire(t, f.publishedState(t))
	tests := []struct {
		name  string
		event creature.Event
	}{
		{name: "registered", event: registered},
		{name: "published", event: published},
		{name: "retired", event: retired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, 1, tt.event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", tt.event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(tt.event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := creature.NewEventType(tt.event.EventType()).Unmarshal(tt.event.GetMeta(), tt.event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, tt.event, restored)
		})
	}
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := creature.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(creature.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}

var changedAt = registeredAt.Add(time.Hour)

// 状態を変えるコマンドを Decide し、できたイベントを Evolve で当てる。変わらない操作ではイベントが無い
func (f fixture) decide(t *testing.T, state creature.State, cmd creature.Command) (creature.State, []creature.Event) {
	t.Helper()
	events, err := creature.Decide(state, cmd, changedAt)
	require.NoError(t, err)
	for _, event := range events {
		state, err = creature.Evolve(state, event)
		require.NoError(t, err)
	}
	return state, events
}

func (f fixture) publish(t *testing.T, state creature.State) (creature.State, creature.Event) {
	t.Helper()
	next, events := f.decide(t, state, &creature.PublishCommand{PublishedBy: f.registeredBy})
	require.Len(t, events, 1)
	return next, events[0]
}

func (f fixture) retire(t *testing.T, state creature.State) (creature.State, creature.Event) {
	t.Helper()
	next, events := f.decide(t, state, &creature.RetireCommand{RetiredBy: f.registeredBy})
	require.Len(t, events, 1)
	return next, events[0]
}

func (f fixture) publishedState(t *testing.T) creature.State {
	t.Helper()
	registered, _ := f.register(t)
	published, _ := f.publish(t, registered)
	return published
}

func (f fixture) retiredState(t *testing.T) creature.State {
	t.Helper()
	retired, _ := f.retire(t, f.publishedState(t))
	return retired
}

func TestDecide_PublishEmitsPublishedEventFromDraftAndRetired(t *testing.T) {
	tests := []struct {
		name  string
		state func(t *testing.T, f fixture) creature.State
		from  int64
	}{
		{name: "draft", state: func(t *testing.T, f fixture) creature.State { s, _ := f.register(t); return s }, from: 1},
		{name: "retired", state: func(t *testing.T, f fixture) creature.State { return f.retiredState(t) }, from: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)

			state, event := f.publish(t, tt.state(t, f))

			require.Equal(t, &creature.PublishedEvent{
				EventMeta:   creature.EventMeta{EntityID: f.id, AggregateVersion: tt.from + 1, OccurredAt: changedAt},
				PublishedBy: f.registeredBy,
			}, event)
			require.Equal(t, card.StatusPublished, state.(creature.Creature).Status)
			require.Equal(t, tt.from+1, state.(creature.Creature).Version)
		})
	}
}

func TestDecide_RetireEmitsRetiredEventFromPublished(t *testing.T) {
	f := newFixture(t)

	state, event := f.retire(t, f.publishedState(t))

	require.Equal(t, &creature.RetiredEvent{
		EventMeta: creature.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		RetiredBy: f.registeredBy,
	}, event)
	require.Equal(t, card.StatusRetired, state.(creature.Creature).Status)
	require.Equal(t, int64(3), state.(creature.Creature).Version)
}

// 変わらない操作は、イベントを作らない（版を進めず、画面の版を古くしないため）
func TestDecide_StatusCommandsEmitNothingWhenStatusIsUnchanged(t *testing.T) {
	tests := []struct {
		name  string
		state func(t *testing.T, f fixture) creature.State
		cmd   creature.Command
	}{
		{name: "publish_published", state: func(t *testing.T, f fixture) creature.State { return f.publishedState(t) }, cmd: &creature.PublishCommand{}},
		{name: "retire_retired", state: func(t *testing.T, f fixture) creature.State { return f.retiredState(t) }, cmd: &creature.RetireCommand{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)

			events, err := creature.Decide(tt.state(t, f), tt.cmd, changedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

func TestDecide_RejectsRetireWhenDraft(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	_, err := creature.Decide(registered, &creature.RetireCommand{RetiredBy: f.registeredBy}, changedAt)

	require.ErrorIs(t, err, card.ErrInvalidStatusTransition)
}

func TestDecide_RejectsStatusCommandsBeforeRegistered(t *testing.T) {
	tests := []struct {
		name string
		cmd  creature.Command
	}{
		{name: "publish", cmd: &creature.PublishCommand{}},
		{name: "retire", cmd: &creature.RetireCommand{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := creature.Decide(creature.InitialState(), tt.cmd, changedAt)

			require.ErrorIs(t, err, creature.ErrInvalidState)
		})
	}
}
