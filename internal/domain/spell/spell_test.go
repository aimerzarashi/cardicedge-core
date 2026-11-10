package spell_test

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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell/spelltest"
)

var (
	registeredAt = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
)

type fixture struct {
	id           spell.ID
	name         card.Name
	registeredBy operator.ID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{
		id:           spelltest.ID(t, 20),
		name:         cardtest.Name(t, "ファイアボール"),
		registeredBy: operatortest.ID(t, 10),
	}
}

func (f fixture) registerCommand() *spell.RegisterCommand {
	return &spell.RegisterCommand{ID: f.id, PublicID: publicID, Name: f.name, RegisteredBy: f.registeredBy}
}

func (f fixture) register(t *testing.T) (spell.State, spell.Event) {
	t.Helper()
	events, err := spell.Decide(spell.InitialState(), f.registerCommand(), registeredAt)
	require.NoError(t, err)
	require.Len(t, events, 1)
	state, err := spell.Evolve(spell.InitialState(), events[0])
	require.NoError(t, err)
	return state, events[0]
}

func TestDecide_RegisterEmitsRegisteredEvent(t *testing.T) {
	f := newFixture(t)

	_, event := f.register(t)

	require.Equal(t, &spell.RegisteredEvent{
		EventMeta:    spell.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:     publicID,
		Name:         f.name,
		Cost:         card.DefaultCost,
		Rarity:       card.DefaultRarity,
		Effect:       spell.NoEffect(),
		RegisteredBy: f.registeredBy,
	}, event)
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	_, err := spell.Decide(registered, f.registerCommand(), registeredAt)

	require.ErrorIs(t, err, spell.ErrInvalidState)
}

func TestEvolve_RegisteredEventMakesSpell(t *testing.T) {
	f := newFixture(t)

	registered, _ := f.register(t)

	require.Equal(t, spell.Spell{
		ID: f.id, Version: 1, PublicID: publicID, Status: card.StatusDraft,
		Cost: card.DefaultCost, Rarity: card.DefaultRarity, Effect: spell.NoEffect(),
	}, registered)
}

func TestEvolve_RejectsRegisteredEventWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)
	registered, event := f.register(t)

	_, err := spell.Evolve(registered, event)

	require.ErrorIs(t, err, spell.ErrInvalidState)
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	f := newFixture(t)
	_, registered := f.register(t)
	state, _ := f.register(t)
	_, published := f.publish(t, state)
	_, retired := f.retire(t, f.publishedState(t))
	_, costChanged := f.decide(t, state, &spell.ChangeCostCommand{Cost: cardtest.Cost(t, 30), ChangedBy: f.registeredBy})
	_, rarityChanged := f.decide(t, state, &spell.ChangeRarityCommand{Rarity: card.RarityStrange, ChangedBy: f.registeredBy})
	_, effectChanged := f.decide(t, state, &spell.ChangeEffectCommand{Effect: spelltest.DrawCards(t, 2), ChangedBy: f.registeredBy})
	tests := []struct {
		name  string
		event spell.Event
	}{
		{name: "registered", event: registered},
		{name: "published", event: published},
		{name: "retired", event: retired},
		{name: "cost_changed", event: costChanged[0]},
		{name: "rarity_changed", event: rarityChanged[0]},
		{name: "effect_changed", event: effectChanged[0]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, 1, tt.event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", tt.event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(tt.event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := spell.NewEventType(tt.event.EventType()).Unmarshal(tt.event.GetMeta(), tt.event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, tt.event, restored)
		})
	}
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := spell.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(spell.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}

var changedAt = registeredAt.Add(time.Hour)

// 状態を変えるコマンドを Decide し、できたイベントを Evolve で当てる。変わらない操作ではイベントが無い
func (f fixture) decide(t *testing.T, state spell.State, cmd spell.Command) (spell.State, []spell.Event) {
	t.Helper()
	events, err := spell.Decide(state, cmd, changedAt)
	require.NoError(t, err)
	for _, event := range events {
		state, err = spell.Evolve(state, event)
		require.NoError(t, err)
	}
	return state, events
}

func (f fixture) publish(t *testing.T, state spell.State) (spell.State, spell.Event) {
	t.Helper()
	next, events := f.decide(t, state, &spell.PublishCommand{PublishedBy: f.registeredBy})
	require.Len(t, events, 1)
	return next, events[0]
}

func (f fixture) retire(t *testing.T, state spell.State) (spell.State, spell.Event) {
	t.Helper()
	next, events := f.decide(t, state, &spell.RetireCommand{RetiredBy: f.registeredBy})
	require.Len(t, events, 1)
	return next, events[0]
}

func (f fixture) publishedState(t *testing.T) spell.State {
	t.Helper()
	registered, _ := f.register(t)
	published, _ := f.publish(t, registered)
	return published
}

func (f fixture) retiredState(t *testing.T) spell.State {
	t.Helper()
	retired, _ := f.retire(t, f.publishedState(t))
	return retired
}

func TestDecide_PublishEmitsPublishedEventFromDraftAndRetired(t *testing.T) {
	tests := []struct {
		name  string
		state func(t *testing.T, f fixture) spell.State
		from  int64
	}{
		{name: "draft", state: func(t *testing.T, f fixture) spell.State { s, _ := f.register(t); return s }, from: 1},
		{name: "retired", state: func(t *testing.T, f fixture) spell.State { return f.retiredState(t) }, from: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)

			state, event := f.publish(t, tt.state(t, f))

			require.Equal(t, &spell.PublishedEvent{
				EventMeta:   spell.EventMeta{EntityID: f.id, AggregateVersion: tt.from + 1, OccurredAt: changedAt},
				PublishedBy: f.registeredBy,
			}, event)
			require.Equal(t, card.StatusPublished, state.(spell.Spell).Status)
			require.Equal(t, tt.from+1, state.(spell.Spell).Version)
		})
	}
}

func TestDecide_RetireEmitsRetiredEventFromPublished(t *testing.T) {
	f := newFixture(t)

	state, event := f.retire(t, f.publishedState(t))

	require.Equal(t, &spell.RetiredEvent{
		EventMeta: spell.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		RetiredBy: f.registeredBy,
	}, event)
	require.Equal(t, card.StatusRetired, state.(spell.Spell).Status)
	require.Equal(t, int64(3), state.(spell.Spell).Version)
}

// 変わらない操作は、イベントを作らない（版を進めず、画面の版を古くしないため）
func TestDecide_StatusCommandsEmitNothingWhenStatusIsUnchanged(t *testing.T) {
	tests := []struct {
		name  string
		state func(t *testing.T, f fixture) spell.State
		cmd   spell.Command
	}{
		{name: "publish_published", state: func(t *testing.T, f fixture) spell.State { return f.publishedState(t) }, cmd: &spell.PublishCommand{}},
		{name: "retire_retired", state: func(t *testing.T, f fixture) spell.State { return f.retiredState(t) }, cmd: &spell.RetireCommand{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)

			events, err := spell.Decide(tt.state(t, f), tt.cmd, changedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

func TestDecide_RejectsRetireWhenDraft(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	_, err := spell.Decide(registered, &spell.RetireCommand{RetiredBy: f.registeredBy}, changedAt)

	require.ErrorIs(t, err, card.ErrInvalidStatusTransition)
}

func TestDecide_RejectsStatusCommandsBeforeRegistered(t *testing.T) {
	tests := []struct {
		name string
		cmd  spell.Command
	}{
		{name: "publish", cmd: &spell.PublishCommand{}},
		{name: "retire", cmd: &spell.RetireCommand{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := spell.Decide(spell.InitialState(), tt.cmd, changedAt)

			require.ErrorIs(t, err, spell.ErrInvalidState)
		})
	}
}

func TestDecide_SpecChangesEmitChangedEventsWhenDraft(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	afterCost, costEvents := f.decide(t, registered, &spell.ChangeCostCommand{Cost: cardtest.Cost(t, 30), ChangedBy: f.registeredBy})
	afterRarity, rarityEvents := f.decide(t, afterCost, &spell.ChangeRarityCommand{Rarity: card.RarityRare, ChangedBy: f.registeredBy})
	afterEffect, effectEvents := f.decide(t, afterRarity, &spell.ChangeEffectCommand{Effect: spelltest.DrawCards(t, 2), ChangedBy: f.registeredBy})

	require.Equal(t, []spell.Event{&spell.CostChangedEvent{
		EventMeta: spell.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Cost:      cardtest.Cost(t, 30),
		ChangedBy: f.registeredBy,
	}}, costEvents)
	require.Equal(t, []spell.Event{&spell.RarityChangedEvent{
		EventMeta: spell.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Rarity:    card.RarityRare,
		ChangedBy: f.registeredBy,
	}}, rarityEvents)
	require.Equal(t, []spell.Event{&spell.EffectChangedEvent{
		EventMeta: spell.EventMeta{EntityID: f.id, AggregateVersion: 4, OccurredAt: changedAt},
		Effect:    spelltest.DrawCards(t, 2),
		ChangedBy: f.registeredBy,
	}}, effectEvents)
	require.Equal(t, spell.Spell{
		ID: f.id, Version: 4, PublicID: publicID, Status: card.StatusDraft,
		Cost: cardtest.Cost(t, 30), Rarity: card.RarityRare, Effect: spelltest.DrawCards(t, 2),
	}, afterEffect)
}

// 同じ値への変更は、イベントを作らない（版を進めず、画面の版を古くしないため）
func TestDecide_SpecChangesEmitNothingWhenUnchanged(t *testing.T) {
	tests := []struct {
		name string
		cmd  spell.Command
	}{
		{name: "same_cost", cmd: &spell.ChangeCostCommand{Cost: card.DefaultCost}},
		{name: "same_rarity", cmd: &spell.ChangeRarityCommand{Rarity: card.DefaultRarity}},
		{name: "same_effect", cmd: &spell.ChangeEffectCommand{Effect: spell.NoEffect()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			registered, _ := f.register(t)

			events, err := spell.Decide(registered, tt.cmd, changedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

// 公開中・使用不可の性能は変えられない（デッキに入れた・game に写した性能が途中で変わるため）
func TestDecide_RejectsSpecChangesWhenNotDraft(t *testing.T) {
	tests := []struct {
		name  string
		state func(t *testing.T, f fixture) spell.State
		cmd   spell.Command
	}{
		{name: "cost_published", state: func(t *testing.T, f fixture) spell.State { return f.publishedState(t) }, cmd: &spell.ChangeCostCommand{Cost: cardtest.Cost(t, 30)}},
		{name: "rarity_retired", state: func(t *testing.T, f fixture) spell.State { return f.retiredState(t) }, cmd: &spell.ChangeRarityCommand{Rarity: card.RarityRare}},
		{name: "effect_published", state: func(t *testing.T, f fixture) spell.State { return f.publishedState(t) }, cmd: &spell.ChangeEffectCommand{Effect: spelltest.DrawCards(t, 1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)

			_, err := spell.Decide(tt.state(t, f), tt.cmd, changedAt)

			require.ErrorIs(t, err, card.ErrNotDraft)
		})
	}
}

func TestDecide_RejectsSpecChangesBeforeRegistered(t *testing.T) {
	tests := []struct {
		name string
		cmd  spell.Command
	}{
		{name: "cost", cmd: &spell.ChangeCostCommand{}},
		{name: "rarity", cmd: &spell.ChangeRarityCommand{}},
		{name: "effect", cmd: &spell.ChangeEffectCommand{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := spell.Decide(spell.InitialState(), tt.cmd, changedAt)

			require.ErrorIs(t, err, spell.ErrInvalidState)
		})
	}
}

func TestNewDrawCount_RejectsLessThanOne(t *testing.T) {
	_, err := spell.NewDrawCount(0)

	require.ErrorIs(t, err, spell.ErrDrawCountTooSmall)
}

func TestEffect_EqualComparesTheContents(t *testing.T) {
	require.True(t, spelltest.DrawCards(t, 1).Equal(spelltest.DrawCards(t, 1)))
	require.False(t, spelltest.DrawCards(t, 1).Equal(spelltest.DrawCards(t, 2)))
	require.False(t, spelltest.DrawCards(t, 1).Equal(spell.NoEffect()))
	require.True(t, spell.NoEffect().Equal(spell.NoEffect()))
}
