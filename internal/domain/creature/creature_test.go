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
		EventMeta:        creature.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:         publicID,
		Name:             f.name,
		Cost:             card.DefaultCost,
		Rarity:           card.DefaultRarity,
		Elements:         creature.DefaultElements(),
		Attack:           creature.DefaultAttack,
		HitPoints:        creature.DefaultHitPoints,
		ItemRestrictions: creature.DefaultItemRestrictions(), PlacementRestrictions: creature.DefaultPlacementRestrictions(), Abilities: creature.DefaultAbilities(),
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
		ID: f.id, Version: 1, PublicID: publicID, Status: card.StatusDraft,
		Cost: card.DefaultCost, Rarity: card.DefaultRarity,
		Elements: creature.DefaultElements(), Attack: creature.DefaultAttack, HitPoints: creature.DefaultHitPoints, ItemRestrictions: creature.DefaultItemRestrictions(), PlacementRestrictions: creature.DefaultPlacementRestrictions(), Abilities: creature.DefaultAbilities(),
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
	_, costChanged := f.decide(t, state, &creature.ChangeCostCommand{Cost: cardtest.Cost(t, 30), ChangedBy: f.registeredBy})
	_, rarityChanged := f.decide(t, state, &creature.ChangeRarityCommand{Rarity: card.RarityStrange, ChangedBy: f.registeredBy})
	_, elementsChanged := f.decide(t, state, &creature.ChangeElementsCommand{Elements: creaturetest.Elements(t, "wind", "fire"), ChangedBy: f.registeredBy})
	_, statsChanged := f.decide(t, state, &creature.ChangeStatsCommand{Attack: creaturetest.Attack(t, 30), HitPoints: creaturetest.HitPoints(t, 50), ChangedBy: f.registeredBy})
	_, abilitiesChanged := f.decide(t, state, &creature.ChangeAbilitiesCommand{Abilities: defensiveAbilities(t), ChangedBy: f.registeredBy})
	_, itemRestrictionsChanged := f.decide(t, state, &creature.ChangeItemRestrictionsCommand{ItemRestrictions: creaturetest.ItemRestrictions(t, "armor", "scroll"), ChangedBy: f.registeredBy})
	_, placementRestrictionsChanged := f.decide(t, state, &creature.ChangePlacementRestrictionsCommand{PlacementRestrictions: creaturetest.PlacementRestrictions(t, "fire", "wind"), ChangedBy: f.registeredBy})
	tests := []struct {
		name  string
		event creature.Event
	}{
		{name: "registered", event: registered},
		{name: "published", event: published},
		{name: "retired", event: retired},
		{name: "cost_changed", event: costChanged[0]},
		{name: "rarity_changed", event: rarityChanged[0]},
		{name: "elements_changed", event: elementsChanged[0]},
		{name: "stats_changed", event: statsChanged[0]},
		{name: "abilities_changed", event: abilitiesChanged[0]},
		{name: "item_restrictions_changed", event: itemRestrictionsChanged[0]},
		{name: "placement_restrictions_changed", event: placementRestrictionsChanged[0]},
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

func TestDecide_SpecChangesEmitChangedEventsWhenDraft(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	afterCost, costEvents := f.decide(t, registered, &creature.ChangeCostCommand{Cost: cardtest.Cost(t, 30), ChangedBy: f.registeredBy})
	afterRarity, rarityEvents := f.decide(t, afterCost, &creature.ChangeRarityCommand{Rarity: card.RarityRare, ChangedBy: f.registeredBy})

	require.Equal(t, []creature.Event{&creature.CostChangedEvent{
		EventMeta: creature.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Cost:      cardtest.Cost(t, 30),
		ChangedBy: f.registeredBy,
	}}, costEvents)
	require.Equal(t, []creature.Event{&creature.RarityChangedEvent{
		EventMeta: creature.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Rarity:    card.RarityRare,
		ChangedBy: f.registeredBy,
	}}, rarityEvents)
	require.Equal(t, creature.Creature{
		ID: f.id, Version: 3, PublicID: publicID, Status: card.StatusDraft,
		Cost: cardtest.Cost(t, 30), Rarity: card.RarityRare,
		Elements: creature.DefaultElements(), Attack: creature.DefaultAttack, HitPoints: creature.DefaultHitPoints, ItemRestrictions: creature.DefaultItemRestrictions(), PlacementRestrictions: creature.DefaultPlacementRestrictions(), Abilities: creature.DefaultAbilities(),
	}, afterRarity)
}

// 同じ値への変更は、イベントを作らない（版を進めず、画面の版を古くしないため）
func TestDecide_SpecChangesEmitNothingWhenUnchanged(t *testing.T) {
	tests := []struct {
		name string
		cmd  creature.Command
	}{
		{name: "same_cost", cmd: &creature.ChangeCostCommand{Cost: card.DefaultCost}},
		{name: "same_rarity", cmd: &creature.ChangeRarityCommand{Rarity: card.DefaultRarity}},
		{name: "same_elements", cmd: &creature.ChangeElementsCommand{Elements: creature.DefaultElements()}},
		{name: "same_stats", cmd: &creature.ChangeStatsCommand{Attack: creature.DefaultAttack, HitPoints: creature.DefaultHitPoints}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			registered, _ := f.register(t)

			events, err := creature.Decide(registered, tt.cmd, changedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

// 公開中・使用不可の性能は変えられない（デッキに入れた・game に写した性能が途中で変わるため）
func TestDecide_RejectsSpecChangesWhenNotDraft(t *testing.T) {
	tests := []struct {
		name  string
		state func(t *testing.T, f fixture) creature.State
		cmd   creature.Command
	}{
		{name: "cost_published", state: func(t *testing.T, f fixture) creature.State { return f.publishedState(t) }, cmd: &creature.ChangeCostCommand{Cost: cardtest.Cost(t, 30)}},
		{name: "rarity_retired", state: func(t *testing.T, f fixture) creature.State { return f.retiredState(t) }, cmd: &creature.ChangeRarityCommand{Rarity: card.RarityRare}},
		{name: "elements_published", state: func(t *testing.T, f fixture) creature.State { return f.publishedState(t) }, cmd: &creature.ChangeElementsCommand{Elements: creaturetest.Elements(t, "fire")}},
		{name: "stats_retired", state: func(t *testing.T, f fixture) creature.State { return f.retiredState(t) }, cmd: &creature.ChangeStatsCommand{Attack: creaturetest.Attack(t, 30), HitPoints: creaturetest.HitPoints(t, 50)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)

			_, err := creature.Decide(tt.state(t, f), tt.cmd, changedAt)

			require.ErrorIs(t, err, card.ErrNotDraft)
		})
	}
}

func TestDecide_RejectsSpecChangesBeforeRegistered(t *testing.T) {
	tests := []struct {
		name string
		cmd  creature.Command
	}{
		{name: "cost", cmd: &creature.ChangeCostCommand{}},
		{name: "rarity", cmd: &creature.ChangeRarityCommand{}},
		{name: "elements", cmd: &creature.ChangeElementsCommand{}},
		{name: "stats", cmd: &creature.ChangeStatsCommand{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := creature.Decide(creature.InitialState(), tt.cmd, changedAt)

			require.ErrorIs(t, err, creature.ErrInvalidState)
		})
	}
}

func TestDecide_ChangeElementsAndStatsEmitChangedEventsWhenDraft(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	afterElements, elementsEvents := f.decide(t, registered, &creature.ChangeElementsCommand{Elements: creaturetest.Elements(t, "water", "fire"), ChangedBy: f.registeredBy})
	afterStats, statsEvents := f.decide(t, afterElements, &creature.ChangeStatsCommand{Attack: creaturetest.Attack(t, 0), HitPoints: creaturetest.HitPoints(t, 60), ChangedBy: f.registeredBy})

	require.Equal(t, []creature.Event{&creature.ElementsChangedEvent{
		EventMeta: creature.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Elements:  creaturetest.Elements(t, "fire", "water"),
		ChangedBy: f.registeredBy,
	}}, elementsEvents)
	require.Equal(t, []creature.Event{&creature.StatsChangedEvent{
		EventMeta: creature.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Attack:    creaturetest.Attack(t, 0),
		HitPoints: creaturetest.HitPoints(t, 60),
		ChangedBy: f.registeredBy,
	}}, statsEvents)
	require.Equal(t, creature.Creature{
		ID: f.id, Version: 3, PublicID: publicID, Status: card.StatusDraft,
		Cost: card.DefaultCost, Rarity: card.DefaultRarity,
		Elements: creaturetest.Elements(t, "fire", "water"), Attack: creaturetest.Attack(t, 0), HitPoints: creaturetest.HitPoints(t, 60), ItemRestrictions: creature.DefaultItemRestrictions(), PlacementRestrictions: creature.DefaultPlacementRestrictions(), Abilities: creature.DefaultAbilities(),
	}, afterStats)
}

// 片方の値だけ変わるときも、変わらない側の値を含めて 1 つのイベントにする
func TestDecide_ChangeStatsEmitsEventWhenOnlyOneValueChanges(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	_, events := f.decide(t, registered, &creature.ChangeStatsCommand{Attack: creature.DefaultAttack, HitPoints: creaturetest.HitPoints(t, 11), ChangedBy: f.registeredBy})

	require.Len(t, events, 1)
	require.Equal(t, creaturetest.HitPoints(t, 11), events[0].(*creature.StatsChangedEvent).HitPoints)
}

func defensiveAbilities(t *testing.T) creature.Abilities {
	t.Helper()
	abilities, err := creature.NewAbilities([]creature.Ability{creature.NewDefensiveAbility()})
	require.NoError(t, err)
	return abilities
}

func TestDecide_ChangeAbilitiesEmitsEventOnlyWhenDraftAndChanged(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	changed, events := f.decide(t, registered, &creature.ChangeAbilitiesCommand{Abilities: defensiveAbilities(t), ChangedBy: f.registeredBy})

	require.Len(t, events, 1)
	require.True(t, changed.(creature.Creature).Abilities.IsDefensive())
	_, same := f.decide(t, registered, &creature.ChangeAbilitiesCommand{Abilities: creature.DefaultAbilities(), ChangedBy: f.registeredBy})
	require.Empty(t, same)
	_, err := creature.Decide(f.retiredState(t), &creature.ChangeAbilitiesCommand{Abilities: defensiveAbilities(t), ChangedBy: f.registeredBy}, changedAt)
	require.ErrorIs(t, err, card.ErrNotDraft)
}

func TestAbilities_TollMultiplierChangesTheTollPercentAndIsComparedByValue(t *testing.T) {
	percent := func(value int) creature.Abilities {
		p, err := creature.NewTollMultiplierPercent(value)
		require.NoError(t, err)
		abilities, err := creature.NewAbilities([]creature.Ability{creature.NewTollMultiplierAbility(p)})
		require.NoError(t, err)
		return abilities
	}

	require.Equal(t, 100, creature.DefaultAbilities().TollPercent())
	require.Equal(t, 150, percent(150).TollPercent())
	require.True(t, percent(150).Equal(percent(150)))
	require.False(t, percent(150).Equal(percent(200)))
	_, err := creature.NewTollMultiplierPercent(0)
	require.ErrorIs(t, err, creature.ErrTollMultiplierPercentTooSmall)
}

func TestNewAbilities_RejectsEmptyKindAndDuplicates(t *testing.T) {
	empty, err := creature.NewAbilities(nil)
	require.NoError(t, err)
	require.NotNil(t, empty)
	require.False(t, empty.IsDefensive())

	_, err = creature.NewAbilities([]creature.Ability{{}})
	require.ErrorIs(t, err, creature.ErrAbilityEmpty)
	_, err = creature.NewAbilities([]creature.Ability{creature.NewDefensiveAbility(), creature.NewDefensiveAbility()})
	require.ErrorIs(t, err, creature.ErrAbilitiesDuplicated)
}

func TestDecide_ChangeItemRestrictionsEmitsEventOnlyWhenDraftAndChanged(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	changed, events := f.decide(t, registered, &creature.ChangeItemRestrictionsCommand{ItemRestrictions: creaturetest.ItemRestrictions(t, "scroll"), ChangedBy: f.registeredBy})

	require.Len(t, events, 1)
	require.Equal(t, creaturetest.ItemRestrictions(t, "scroll"), changed.(creature.Creature).ItemRestrictions)
	_, same := f.decide(t, registered, &creature.ChangeItemRestrictionsCommand{ItemRestrictions: creature.DefaultItemRestrictions(), ChangedBy: f.registeredBy})
	require.Empty(t, same)
	_, err := creature.Decide(f.retiredState(t), &creature.ChangeItemRestrictionsCommand{ItemRestrictions: creaturetest.ItemRestrictions(t, "scroll"), ChangedBy: f.registeredBy}, changedAt)
	require.ErrorIs(t, err, card.ErrNotDraft)
}

func TestNewItemRestrictions_SortsAndRejectsUnknownAndDuplicates(t *testing.T) {
	empty, err := creature.NewItemRestrictions(nil)
	require.NoError(t, err)
	require.NotNil(t, empty)
	require.Empty(t, empty)

	sorted, err := creature.NewItemRestrictions([]string{"scroll", "weapon"})
	require.NoError(t, err)
	require.Equal(t, []string{"weapon", "scroll"}, sorted.Strings())

	_, err = creature.NewItemRestrictions([]string{"tool"})
	require.ErrorIs(t, err, creature.ErrItemRestrictionUnknown)
	_, err = creature.NewItemRestrictions([]string{"armor", "armor"})
	require.ErrorIs(t, err, creature.ErrItemRestrictionsDuplicated)
}

func TestItemRestrictions_ForbidsOnlyTheListedCategories(t *testing.T) {
	restrictions := creaturetest.ItemRestrictions(t, "armor")

	require.True(t, restrictions.Forbids("armor"))
	require.False(t, restrictions.Forbids("weapon"))
}

func TestDecide_ChangePlacementRestrictionsEmitsEventOnlyWhenDraftAndChanged(t *testing.T) {
	f := newFixture(t)
	registered, _ := f.register(t)

	changed, events := f.decide(t, registered, &creature.ChangePlacementRestrictionsCommand{PlacementRestrictions: creaturetest.PlacementRestrictions(t, "water"), ChangedBy: f.registeredBy})

	require.Len(t, events, 1)
	require.Equal(t, creaturetest.PlacementRestrictions(t, "water"), changed.(creature.Creature).PlacementRestrictions)
	_, same := f.decide(t, registered, &creature.ChangePlacementRestrictionsCommand{PlacementRestrictions: creature.DefaultPlacementRestrictions(), ChangedBy: f.registeredBy})
	require.Empty(t, same)
	_, err := creature.Decide(f.retiredState(t), &creature.ChangePlacementRestrictionsCommand{PlacementRestrictions: creaturetest.PlacementRestrictions(t, "water"), ChangedBy: f.registeredBy}, changedAt)
	require.ErrorIs(t, err, card.ErrNotDraft)
}

func TestNewPlacementRestrictions_SortsAndRejectsNeutralUnknownAndDuplicates(t *testing.T) {
	empty, err := creature.NewPlacementRestrictions(nil)
	require.NoError(t, err)
	require.NotNil(t, empty)
	require.Empty(t, empty)

	sorted, err := creature.NewPlacementRestrictions([]string{"wind", "fire"})
	require.NoError(t, err)
	require.Equal(t, []string{"fire", "wind"}, sorted.Strings())

	_, err = creature.NewPlacementRestrictions([]string{"neutral"})
	require.ErrorIs(t, err, creature.ErrPlacementRestrictionUnknown)
	_, err = creature.NewPlacementRestrictions([]string{"multi"})
	require.ErrorIs(t, err, creature.ErrPlacementRestrictionUnknown)
	_, err = creature.NewPlacementRestrictions([]string{"fire", "fire"})
	require.ErrorIs(t, err, creature.ErrPlacementRestrictionsDuplicated)
}

func TestPlacementRestrictions_ForbidsOnlyTheListedLandElements(t *testing.T) {
	restrictions := creaturetest.PlacementRestrictions(t, "fire", "earth", "water", "wind")

	require.True(t, restrictions.Forbids("fire"))
	require.False(t, restrictions.Forbids("neutral"))
	require.False(t, restrictions.Forbids("multi"))
}

func TestNewElements_SortsIntoCanonicalOrder(t *testing.T) {
	elements, err := creature.NewElements([]string{"wind", "water", "fire", "earth"})

	require.NoError(t, err)
	require.Equal(t, []string{"fire", "earth", "water", "wind"}, elements.Strings())
}

func TestNewElements_AcceptsNeutralAlone(t *testing.T) {
	elements, err := creature.NewElements([]string{"neutral"})

	require.NoError(t, err)
	require.True(t, elements.Equal(creature.DefaultElements()))
}

func TestNewElements_RejectsInvalidSets(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   error
	}{
		{name: "empty", values: nil, want: creature.ErrElementsEmpty},
		{name: "unknown", values: []string{"light"}, want: creature.ErrElementUnknown},
		{name: "neutral_mixed", values: []string{"neutral", "fire"}, want: creature.ErrElementsNeutralMixed},
		{name: "duplicated", values: []string{"fire", "fire"}, want: creature.ErrElementsDuplicated},
		{name: "duplicated_neutral", values: []string{"neutral", "neutral"}, want: creature.ErrElementsDuplicated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := creature.NewElements(tt.values)

			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestNewAttack_AcceptsZeroAndRejectsNegative(t *testing.T) {
	zero, err := creature.NewAttack(0)
	require.NoError(t, err)
	require.Equal(t, 0, zero.Int())

	_, err = creature.NewAttack(-1)
	require.ErrorIs(t, err, creature.ErrAttackNegative)
}

func TestNewHitPoints_AcceptsOneAndRejectsZero(t *testing.T) {
	one, err := creature.NewHitPoints(1)
	require.NoError(t, err)
	require.Equal(t, 1, one.Int())

	_, err = creature.NewHitPoints(0)
	require.ErrorIs(t, err, creature.ErrHitPointsTooSmall)
}

func TestHitPoints_DamagedSubtractsTheAttackAndStopsAtZero(t *testing.T) {
	hitPoints := creaturetest.HitPoints(t, 20)

	require.Equal(t, 15, hitPoints.Damaged(creaturetest.Attack(t, 5)).Int())
	require.Equal(t, 0, hitPoints.Damaged(creaturetest.Attack(t, 20)).Int())
	require.Equal(t, 0, hitPoints.Damaged(creaturetest.Attack(t, 30)).Int())
}
