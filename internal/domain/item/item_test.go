package item_test

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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item/itemtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
)

var (
	registeredAt = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
)

type fixture struct {
	id           item.ID
	name         card.Name
	registeredBy operator.ID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{
		id:           itemtest.ID(t, 20),
		name:         cardtest.Name(t, "ポーション"),
		registeredBy: operatortest.ID(t, 10),
	}
}

func (f fixture) registerCommand() *item.RegisterCommand {
	return &item.RegisterCommand{ID: f.id, PublicID: publicID, Name: f.name, RegisteredBy: f.registeredBy}
}

func (f fixture) register(t *testing.T) (item.State, item.Event) {
	t.Helper()
	events, err := item.Decide(item.InitialState(), f.registerCommand(), registeredAt)
	require.NoError(t, err)
	require.Len(t, events, 1)
	state, err := item.Evolve(item.InitialState(), events[0])
	require.NoError(t, err)
	return state, events[0]
}

func TestDecide_RegisterEmitsRegisteredEvent(t *testing.T) {
	f := newFixture(t)

	_, event := f.register(t)

	require.Equal(t, &item.RegisteredEvent{
		EventMeta:    item.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:     publicID,
		Name:         f.name,
		Cost:         card.DefaultCost,
		Rarity:       card.DefaultRarity,
		RegisteredBy: f.registeredBy,
	}, event)
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	_, err := item.Decide(registered, f.registerCommand(), registeredAt)

	require.ErrorIs(t, err, item.ErrInvalidState)
}

func TestEvolve_RegisteredEventMakesItem(t *testing.T) {
	f := newFixture(t)

	registered, _ := f.register(t)

	require.Equal(t, item.Item{
		ID: f.id, Version: 1, PublicID: publicID, Status: card.StatusDraft,
		Cost: card.DefaultCost, Rarity: card.DefaultRarity,
	}, registered)
}

func TestEvolve_RejectsRegisteredEventWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)
	registered, event := f.register(t)

	_, err := item.Evolve(registered, event)

	require.ErrorIs(t, err, item.ErrInvalidState)
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	f := newFixture(t)
	_, registered := f.register(t)
	state, _ := f.register(t)
	_, published := f.publish(t, state)
	_, retired := f.retire(t, f.publishedState(t))
	_, costChanged := f.decide(t, state, &item.ChangeCostCommand{Cost: cardtest.Cost(t, 30), ChangedBy: f.registeredBy})
	_, rarityChanged := f.decide(t, state, &item.ChangeRarityCommand{Rarity: card.RarityStrange, ChangedBy: f.registeredBy})
	tests := []struct {
		name  string
		event item.Event
	}{
		{name: "registered", event: registered},
		{name: "published", event: published},
		{name: "retired", event: retired},
		{name: "cost_changed", event: costChanged[0]},
		{name: "rarity_changed", event: rarityChanged[0]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, 1, tt.event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", tt.event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(tt.event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := item.NewEventType(tt.event.EventType()).Unmarshal(tt.event.GetMeta(), tt.event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, tt.event, restored)
		})
	}
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := item.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(item.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}

var changedAt = registeredAt.Add(time.Hour)

// 状態を変えるコマンドを Decide し、できたイベントを Evolve で当てる。変わらない操作ではイベントが無い
func (f fixture) decide(t *testing.T, state item.State, cmd item.Command) (item.State, []item.Event) {
	t.Helper()
	events, err := item.Decide(state, cmd, changedAt)
	require.NoError(t, err)
	for _, event := range events {
		state, err = item.Evolve(state, event)
		require.NoError(t, err)
	}
	return state, events
}

func (f fixture) publish(t *testing.T, state item.State) (item.State, item.Event) {
	t.Helper()
	next, events := f.decide(t, state, &item.PublishCommand{PublishedBy: f.registeredBy})
	require.Len(t, events, 1)
	return next, events[0]
}

func (f fixture) retire(t *testing.T, state item.State) (item.State, item.Event) {
	t.Helper()
	next, events := f.decide(t, state, &item.RetireCommand{RetiredBy: f.registeredBy})
	require.Len(t, events, 1)
	return next, events[0]
}

func (f fixture) publishedState(t *testing.T) item.State {
	t.Helper()
	registered, _ := f.register(t)
	published, _ := f.publish(t, registered)
	return published
}

func (f fixture) retiredState(t *testing.T) item.State {
	t.Helper()
	retired, _ := f.retire(t, f.publishedState(t))
	return retired
}

func TestDecide_PublishEmitsPublishedEventFromDraftAndRetired(t *testing.T) {
	tests := []struct {
		name  string
		state func(t *testing.T, f fixture) item.State
		from  int64
	}{
		{name: "draft", state: func(t *testing.T, f fixture) item.State { s, _ := f.register(t); return s }, from: 1},
		{name: "retired", state: func(t *testing.T, f fixture) item.State { return f.retiredState(t) }, from: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)

			state, event := f.publish(t, tt.state(t, f))

			require.Equal(t, &item.PublishedEvent{
				EventMeta:   item.EventMeta{EntityID: f.id, AggregateVersion: tt.from + 1, OccurredAt: changedAt},
				PublishedBy: f.registeredBy,
			}, event)
			require.Equal(t, card.StatusPublished, state.(item.Item).Status)
			require.Equal(t, tt.from+1, state.(item.Item).Version)
		})
	}
}

func TestDecide_RetireEmitsRetiredEventFromPublished(t *testing.T) {
	f := newFixture(t)

	state, event := f.retire(t, f.publishedState(t))

	require.Equal(t, &item.RetiredEvent{
		EventMeta: item.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		RetiredBy: f.registeredBy,
	}, event)
	require.Equal(t, card.StatusRetired, state.(item.Item).Status)
	require.Equal(t, int64(3), state.(item.Item).Version)
}

// 変わらない操作は、イベントを作らない（版を進めず、画面の版を古くしないため）
func TestDecide_StatusCommandsEmitNothingWhenStatusIsUnchanged(t *testing.T) {
	tests := []struct {
		name  string
		state func(t *testing.T, f fixture) item.State
		cmd   item.Command
	}{
		{name: "publish_published", state: func(t *testing.T, f fixture) item.State { return f.publishedState(t) }, cmd: &item.PublishCommand{}},
		{name: "retire_retired", state: func(t *testing.T, f fixture) item.State { return f.retiredState(t) }, cmd: &item.RetireCommand{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)

			events, err := item.Decide(tt.state(t, f), tt.cmd, changedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

func TestDecide_RejectsRetireWhenDraft(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	_, err := item.Decide(registered, &item.RetireCommand{RetiredBy: f.registeredBy}, changedAt)

	require.ErrorIs(t, err, card.ErrInvalidStatusTransition)
}

func TestDecide_RejectsStatusCommandsBeforeRegistered(t *testing.T) {
	tests := []struct {
		name string
		cmd  item.Command
	}{
		{name: "publish", cmd: &item.PublishCommand{}},
		{name: "retire", cmd: &item.RetireCommand{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := item.Decide(item.InitialState(), tt.cmd, changedAt)

			require.ErrorIs(t, err, item.ErrInvalidState)
		})
	}
}

func TestDecide_SpecChangesEmitChangedEventsWhenDraft(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	afterCost, costEvents := f.decide(t, registered, &item.ChangeCostCommand{Cost: cardtest.Cost(t, 30), ChangedBy: f.registeredBy})
	afterRarity, rarityEvents := f.decide(t, afterCost, &item.ChangeRarityCommand{Rarity: card.RarityRare, ChangedBy: f.registeredBy})

	require.Equal(t, []item.Event{&item.CostChangedEvent{
		EventMeta: item.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Cost:      cardtest.Cost(t, 30),
		ChangedBy: f.registeredBy,
	}}, costEvents)
	require.Equal(t, []item.Event{&item.RarityChangedEvent{
		EventMeta: item.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Rarity:    card.RarityRare,
		ChangedBy: f.registeredBy,
	}}, rarityEvents)
	require.Equal(t, item.Item{
		ID: f.id, Version: 3, PublicID: publicID, Status: card.StatusDraft,
		Cost: cardtest.Cost(t, 30), Rarity: card.RarityRare,
	}, afterRarity)
}

// 同じ値への変更は、イベントを作らない（版を進めず、画面の版を古くしないため）
func TestDecide_SpecChangesEmitNothingWhenUnchanged(t *testing.T) {
	tests := []struct {
		name string
		cmd  item.Command
	}{
		{name: "same_cost", cmd: &item.ChangeCostCommand{Cost: card.DefaultCost}},
		{name: "same_rarity", cmd: &item.ChangeRarityCommand{Rarity: card.DefaultRarity}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			registered, _ := f.register(t)

			events, err := item.Decide(registered, tt.cmd, changedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

// 公開中・使用不可の性能は変えられない（デッキに入れた・game に写した性能が途中で変わるため）
func TestDecide_RejectsSpecChangesWhenNotDraft(t *testing.T) {
	tests := []struct {
		name  string
		state func(t *testing.T, f fixture) item.State
		cmd   item.Command
	}{
		{name: "cost_published", state: func(t *testing.T, f fixture) item.State { return f.publishedState(t) }, cmd: &item.ChangeCostCommand{Cost: cardtest.Cost(t, 30)}},
		{name: "rarity_retired", state: func(t *testing.T, f fixture) item.State { return f.retiredState(t) }, cmd: &item.ChangeRarityCommand{Rarity: card.RarityRare}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)

			_, err := item.Decide(tt.state(t, f), tt.cmd, changedAt)

			require.ErrorIs(t, err, card.ErrNotDraft)
		})
	}
}

func TestDecide_RejectsSpecChangesBeforeRegistered(t *testing.T) {
	tests := []struct {
		name string
		cmd  item.Command
	}{
		{name: "cost", cmd: &item.ChangeCostCommand{}},
		{name: "rarity", cmd: &item.ChangeRarityCommand{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := item.Decide(item.InitialState(), tt.cmd, changedAt)

			require.ErrorIs(t, err, item.ErrInvalidState)
		})
	}
}
