package starterdeck_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck/starterdecktest"
)

var (
	registeredAt = time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	changedAt    = time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
)

type fixture struct {
	id       starterdeck.ID
	name     starterdeck.Name
	operator operator.ID
	dragon   starterdeck.CardRef
	potion   starterdeck.CardRef
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{
		id:       starterdecktest.ID(t, 30),
		name:     starterdecktest.Name(t, "炎のデッキ"),
		operator: operatortest.ID(t, 10),
		dragon:   starterdecktest.CardRef(t, creature.Types().EntityType, 20),
		potion:   starterdecktest.CardRef(t, item.Types().EntityType, 21),
	}
}

func (f fixture) registerCommand() *starterdeck.RegisterCommand {
	return &starterdeck.RegisterCommand{ID: f.id, PublicID: publicID, Name: f.name, RegisteredBy: f.operator}
}

// Decide の結果を Evolve に当てる（コマンドを順に実行したときの状態を作る）
func apply(t *testing.T, state starterdeck.State, cmd starterdeck.Command, now time.Time) (starterdeck.State, []starterdeck.Event) {
	t.Helper()
	events, err := starterdeck.Decide(state, cmd, now)
	require.NoError(t, err)
	for _, event := range events {
		state, err = starterdeck.Evolve(state, event)
		require.NoError(t, err)
	}
	return state, events
}

func (f fixture) register(t *testing.T) starterdeck.State {
	t.Helper()
	state, _ := apply(t, starterdeck.InitialState(), f.registerCommand(), registeredAt)
	return state
}

func (f fixture) setCountsCommand(t *testing.T, counts map[starterdeck.CardRef]int) *starterdeck.SetCardCountsCommand {
	t.Helper()
	cardCounts := make(map[starterdeck.CardRef]starterdeck.CardCount, len(counts))
	for card, count := range counts {
		cardCounts[card] = starterdecktest.CardCount(t, count)
	}
	return &starterdeck.SetCardCountsCommand{Counts: cardCounts, ChangedBy: f.operator}
}

func (f fixture) setCounts(t *testing.T, state starterdeck.State, counts map[starterdeck.CardRef]int) starterdeck.State {
	t.Helper()
	state, _ = apply(t, state, f.setCountsCommand(t, counts), changedAt)
	return state
}

func TestDecide_RegisterEmitsRegisteredEvent(t *testing.T) {
	f := newFixture(t)

	events, err := starterdeck.Decide(starterdeck.InitialState(), f.registerCommand(), registeredAt)

	require.NoError(t, err)
	require.Equal(t, []starterdeck.Event{&starterdeck.RegisteredEvent{
		EventMeta:    starterdeck.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:     publicID,
		Name:         f.name,
		RegisteredBy: f.operator,
	}}, events)
}

func TestEvolve_RegisteredEventMakesEmptyStarterDeck(t *testing.T) {
	f := newFixture(t)

	state := f.register(t)

	require.Equal(t, starterdeck.StarterDeck{
		ID: f.id, Version: 1, PublicID: publicID, Name: f.name, Cards: map[starterdeck.CardRef]starterdeck.CardCount{},
		RegisteredBy: f.operator, RegisteredAt: registeredAt, UpdatedAt: registeredAt,
	}, state)
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)

	_, err := starterdeck.Decide(f.register(t), f.registerCommand(), registeredAt)

	require.ErrorIs(t, err, starterdeck.ErrInvalidState)
}

func TestDecide_RenameChangesNameWithNextVersion(t *testing.T) {
	f := newFixture(t)
	newName := starterdecktest.Name(t, "水のデッキ")

	state, events := apply(t, f.register(t), &starterdeck.RenameCommand{Name: newName, RenamedBy: f.operator}, changedAt)

	require.Equal(t, []starterdeck.Event{&starterdeck.RenamedEvent{
		EventMeta: starterdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Name:      newName,
		RenamedBy: f.operator,
	}}, events)
	deck := state.(starterdeck.StarterDeck)
	require.Equal(t, newName, deck.Name)
	require.Equal(t, int64(2), deck.Version)
	require.Equal(t, changedAt, deck.UpdatedAt)
}

// 版を進めないため（画面で見た版を古くしないため）
func TestDecide_EmitsNothingWhenNothingChanges(t *testing.T) {
	f := newFixture(t)
	withDragon := f.setCounts(t, f.register(t), map[starterdeck.CardRef]int{f.dragon: 2})
	tests := []struct {
		name string
		cmd  starterdeck.Command
	}{
		{name: "same_name", cmd: &starterdeck.RenameCommand{Name: f.name, RenamedBy: f.operator}},
		{name: "same_count", cmd: f.setCountsCommand(t, map[starterdeck.CardRef]int{f.dragon: 2})},
		{name: "zero_count_of_card_not_in_deck", cmd: f.setCountsCommand(t, map[starterdeck.CardRef]int{f.potion: 0})},
		{name: "no_cards", cmd: f.setCountsCommand(t, map[starterdeck.CardRef]int{})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := starterdeck.Decide(withDragon, tt.cmd, changedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

// 変わったカードだけを、差分で、1 つのイベントに入れる
func TestDecide_SetCardCountsEmitsDeltasOfChangedCards(t *testing.T) {
	f := newFixture(t)
	slime := starterdecktest.CardRef(t, creature.Types().EntityType, 22)
	state := f.setCounts(t, f.register(t), map[starterdeck.CardRef]int{f.dragon: 3, slime: 1})

	_, events := apply(t, state, f.setCountsCommand(t, map[starterdeck.CardRef]int{f.potion: 4, f.dragon: 1, slime: 1}), changedAt)

	require.Equal(t, []starterdeck.Event{&starterdeck.CardCountsChangedEvent{
		EventMeta: starterdeck.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Changes: []starterdeck.CardCountChange{
			{Card: f.dragon, Delta: -2},
			{Card: f.potion, Delta: 4},
		},
		ChangedBy: f.operator,
	}}, events)
}

// 合計はちょうど上限まで入れられ、超える変更はイベントを作らない
func TestDecide_RejectsTotalCountOverMax(t *testing.T) {
	f := newFixture(t)
	full := map[starterdeck.CardRef]int{}
	for i := range starterdeck.MaxTotalCount / starterdeck.MaxCardCount {
		full[starterdecktest.CardRef(t, creature.Types().EntityType, int64(100+i))] = starterdeck.MaxCardCount
	}
	state := f.setCounts(t, f.register(t), full)
	require.Equal(t, starterdeck.MaxTotalCount, state.(starterdeck.StarterDeck).TotalCount())

	_, err := starterdeck.Decide(state, f.setCountsCommand(t, map[starterdeck.CardRef]int{f.potion: 1}), changedAt)

	require.ErrorIs(t, err, starterdeck.ErrTotalCountTooLarge)
}

// 0 にするとデッキから外れる（枚数が 0 のカードは持たない）
func TestEvolve_CardCountsChangedEventSetsAndRemovesCards(t *testing.T) {
	f := newFixture(t)
	state := f.setCounts(t, f.register(t), map[starterdeck.CardRef]int{f.dragon: 3, f.potion: 2})
	state = f.setCounts(t, state, map[starterdeck.CardRef]int{f.dragon: 0, f.potion: 4})

	deck := state.(starterdeck.StarterDeck)

	require.Equal(t, map[starterdeck.CardRef]starterdeck.CardCount{f.potion: starterdecktest.CardCount(t, 4)}, deck.Cards)
	require.Equal(t, 4, deck.TotalCount())
	require.Equal(t, int64(3), deck.Version)
}

// 保存済みのイベントが壊れていても、範囲の外の枚数の状態を作らない
func TestEvolve_RejectsDeltaOutOfRange(t *testing.T) {
	f := newFixture(t)
	event := &starterdeck.CardCountsChangedEvent{
		EventMeta: starterdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Changes:   []starterdeck.CardCountChange{{Card: f.dragon, Delta: -1}},
		ChangedBy: f.operator,
	}

	_, err := starterdeck.Evolve(f.register(t), event)

	require.ErrorIs(t, err, starterdeck.ErrCardCountOutOfRange)
}

func TestEvolve_DoesNotChangeTheGivenState(t *testing.T) {
	f := newFixture(t)
	before := f.setCounts(t, f.register(t), map[starterdeck.CardRef]int{f.dragon: 3})

	f.setCounts(t, before, map[starterdeck.CardRef]int{f.dragon: 1})

	require.Equal(t, starterdecktest.CardCount(t, 3), before.(starterdeck.StarterDeck).Cards[f.dragon])
}

func TestDecide_DeleteMakesDeletedStarterDeck(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.register(t), &starterdeck.DeleteCommand{DeletedBy: f.operator}, changedAt)

	require.Equal(t, []starterdeck.Event{&starterdeck.DeletedEvent{
		EventMeta: starterdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		DeletedBy: f.operator,
	}}, events)
	require.Equal(t, starterdeck.DeletedStarterDeck{ID: f.id, Version: 2}, state)
}

func TestDecide_RejectsChangesUnlessRegistered(t *testing.T) {
	f := newFixture(t)
	deleted, _ := apply(t, f.register(t), &starterdeck.DeleteCommand{DeletedBy: f.operator}, changedAt)
	commands := []struct {
		name string
		cmd  starterdeck.Command
	}{
		{name: "rename", cmd: &starterdeck.RenameCommand{Name: starterdecktest.Name(t, "水のデッキ"), RenamedBy: f.operator}},
		{name: "set_card_counts", cmd: f.setCountsCommand(t, map[starterdeck.CardRef]int{f.dragon: 1})},
		{name: "delete", cmd: &starterdeck.DeleteCommand{DeletedBy: f.operator}},
	}
	states := []struct {
		name  string
		state starterdeck.State
	}{
		{name: "unregistered", state: starterdeck.InitialState()},
		{name: "deleted", state: deleted},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := starterdeck.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, starterdeck.ErrInvalidState)
			})
		}
	}
}

func TestNewName_RejectsInvalidName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  error
	}{
		{name: "blank", value: " ", want: starterdeck.ErrNameEmpty},
		{name: "too_long", value: strings.Repeat("あ", starterdeck.MaxNameLength+1), want: starterdeck.ErrNameTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := starterdeck.NewName(tt.value)

			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestNewCardCount_AcceptsZeroToMax(t *testing.T) {
	tests := []struct {
		name  string
		value int
		ok    bool
	}{
		{name: "negative", value: -1, ok: false},
		{name: "zero", value: 0, ok: true},
		{name: "max", value: starterdeck.MaxCardCount, ok: true},
		{name: "over_max", value: starterdeck.MaxCardCount + 1, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := starterdeck.NewCardCount(tt.value)

			if tt.ok {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, starterdeck.ErrCardCountOutOfRange)
		})
	}
}

func TestNewCardRef_RejectsAggregateThatIsNotCard(t *testing.T) {
	_, err := starterdeck.NewCardRef(operator.Types().EntityType, 1)

	require.ErrorIs(t, err, starterdeck.ErrNotCard)
}

// 比べ方はカード名と同じ（全角・半角、大文字・小文字の違いを同じとみなす）
func TestNameKey_TreatsWidthAndCaseAsSame(t *testing.T) {
	require.Equal(t,
		starterdeck.NameKey(starterdecktest.Name(t, "Ｆire デッキ")),
		starterdeck.NameKey(starterdecktest.Name(t, "fire デッキ")))
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	f := newFixture(t)
	meta := starterdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt}
	events := []starterdeck.Event{
		&starterdeck.RegisteredEvent{EventMeta: meta, PublicID: publicID, Name: f.name, RegisteredBy: f.operator},
		&starterdeck.RenamedEvent{EventMeta: meta, Name: starterdecktest.Name(t, "水のデッキ"), RenamedBy: f.operator},
		&starterdeck.CardCountsChangedEvent{EventMeta: meta, Changes: []starterdeck.CardCountChange{
			{Card: f.dragon, Delta: -2},
			{Card: f.potion, Delta: 3},
		}, ChangedBy: f.operator},
		&starterdeck.DeletedEvent{EventMeta: meta, DeletedBy: f.operator},
	}
	for _, event := range events {
		t.Run(event.EventType().String(), func(t *testing.T) {
			require.Equal(t, 1, event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := starterdeck.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, event, restored)
		})
	}
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := starterdeck.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(starterdeck.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}
