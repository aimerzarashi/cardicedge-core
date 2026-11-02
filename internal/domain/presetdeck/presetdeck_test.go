package presetdeck_test

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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck/presetdecktest"
)

var (
	registeredAt = time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	changedAt    = time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
)

type fixture struct {
	id         presetdeck.ID
	name       presetdeck.Name
	operatorID operator.ID
	dragon     presetdeck.CardRef
	potion     presetdeck.CardRef
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{
		id:         presetdecktest.ID(t, 30),
		name:       presetdecktest.Name(t, "炎のデッキ"),
		operatorID: operatortest.ID(t, 10),
		dragon:     presetdecktest.CardRef(t, creature.Types().EntityType, 20),
		potion:     presetdecktest.CardRef(t, item.Types().EntityType, 21),
	}
}

func (f fixture) registerCommand() *presetdeck.RegisterCommand {
	return &presetdeck.RegisterCommand{ID: f.id, PublicID: publicID, Name: f.name, RegisteredBy: f.operatorID}
}

// Decide の結果を Evolve に当てる（コマンドを順に実行したときの状態を作る）
func apply(t *testing.T, state presetdeck.State, cmd presetdeck.Command, now time.Time) (presetdeck.State, []presetdeck.Event) {
	t.Helper()
	events, err := presetdeck.Decide(state, cmd, now)
	require.NoError(t, err)
	for _, event := range events {
		state, err = presetdeck.Evolve(state, event)
		require.NoError(t, err)
	}
	return state, events
}

func (f fixture) register(t *testing.T) presetdeck.State {
	t.Helper()
	state, _ := apply(t, presetdeck.InitialState(), f.registerCommand(), registeredAt)
	return state
}

func (f fixture) setCountsCommand(t *testing.T, counts map[presetdeck.CardRef]int) *presetdeck.SetCardCountsCommand {
	t.Helper()
	cardCounts := make(map[presetdeck.CardRef]presetdeck.CardCount, len(counts))
	for card, count := range counts {
		cardCounts[card] = presetdecktest.CardCount(t, count)
	}
	return &presetdeck.SetCardCountsCommand{Counts: cardCounts, Rules: gamerules.Defaults().Deck, ChangedBy: f.operatorID}
}

func (f fixture) setCounts(t *testing.T, state presetdeck.State, counts map[presetdeck.CardRef]int) presetdeck.State {
	t.Helper()
	state, _ = apply(t, state, f.setCountsCommand(t, counts), changedAt)
	return state
}

func TestDecide_RegisterEmitsRegisteredEvent(t *testing.T) {
	f := newFixture(t)

	events, err := presetdeck.Decide(presetdeck.InitialState(), f.registerCommand(), registeredAt)

	require.NoError(t, err)
	require.Equal(t, []presetdeck.Event{&presetdeck.RegisteredEvent{
		EventMeta:    presetdeck.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:     publicID,
		Name:         f.name,
		RegisteredBy: f.operatorID,
	}}, events)
}

func TestEvolve_RegisteredEventMakesEmptyPresetDeck(t *testing.T) {
	f := newFixture(t)

	state := f.register(t)

	require.Equal(t, presetdeck.PresetDeck{
		ID: f.id, Version: 1, PublicID: publicID, Name: f.name, Cards: map[presetdeck.CardRef]presetdeck.CardCount{},
	}, state)
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)

	_, err := presetdeck.Decide(f.register(t), f.registerCommand(), registeredAt)

	require.ErrorIs(t, err, presetdeck.ErrInvalidState)
}

func TestDecide_RenameChangesNameWithNextVersion(t *testing.T) {
	f := newFixture(t)
	newName := presetdecktest.Name(t, "水のデッキ")

	state, events := apply(t, f.register(t), &presetdeck.RenameCommand{Name: newName, RenamedBy: f.operatorID}, changedAt)

	require.Equal(t, []presetdeck.Event{&presetdeck.RenamedEvent{
		EventMeta: presetdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Name:      newName,
		RenamedBy: f.operatorID,
	}}, events)
	deck := state.(presetdeck.PresetDeck)
	require.Equal(t, newName, deck.Name)
	require.Equal(t, int64(2), deck.Version)
}

// 版を進めないため（画面で見た版を古くしないため）
func TestDecide_EmitsNothingWhenNothingChanges(t *testing.T) {
	f := newFixture(t)
	withDragon := f.setCounts(t, f.register(t), map[presetdeck.CardRef]int{f.dragon: 2})
	tests := []struct {
		name string
		cmd  presetdeck.Command
	}{
		{name: "same_name", cmd: &presetdeck.RenameCommand{Name: f.name, RenamedBy: f.operatorID}},
		{name: "same_count", cmd: f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 2})},
		{name: "zero_count_of_card_not_in_deck", cmd: f.setCountsCommand(t, map[presetdeck.CardRef]int{f.potion: 0})},
		{name: "no_cards", cmd: f.setCountsCommand(t, map[presetdeck.CardRef]int{})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := presetdeck.Decide(withDragon, tt.cmd, changedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

// 変わったカードだけを、差分で、1 つのイベントに入れる
func TestDecide_SetCardCountsEmitsDeltasOfChangedCards(t *testing.T) {
	f := newFixture(t)
	slime := presetdecktest.CardRef(t, creature.Types().EntityType, 22)
	state := f.setCounts(t, f.register(t), map[presetdeck.CardRef]int{f.dragon: 3, slime: 1})

	_, events := apply(t, state, f.setCountsCommand(t, map[presetdeck.CardRef]int{f.potion: 4, f.dragon: 1, slime: 1}), changedAt)

	require.Equal(t, []presetdeck.Event{&presetdeck.CardCountsChangedEvent{
		EventMeta: presetdeck.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Changes: []presetdeck.CardCountChange{
			{Card: f.dragon, Delta: -2},
			{Card: f.potion, Delta: 4},
		},
		ChangedBy: f.operatorID,
	}}, events)
}

// 合計はちょうど上限まで入れられ、超える変更はイベントを作らない。上限はコマンドで渡されたルールの値
func TestDecide_RejectsTotalCountOverRule(t *testing.T) {
	f := newFixture(t)
	state := f.register(t)
	cmd := f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 3, f.potion: 2})
	cmd.Rules.TotalCardCount = 5
	state, _ = apply(t, state, cmd, changedAt)
	more := f.setCountsCommand(t, map[presetdeck.CardRef]int{f.potion: 3})
	more.Rules.TotalCardCount = 5

	_, err := presetdeck.Decide(state, more, changedAt)

	require.ErrorIs(t, err, presetdeck.ErrTotalCountTooLarge)
}

// 同じカードの枚数の上限も、コマンドで渡されたルールの値。ちょうど上限は入れられる
func TestDecide_RejectsCardCountOverRule(t *testing.T) {
	f := newFixture(t)
	atMax := f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 2})
	atMax.Rules.MaxCardDuplicates = 2
	over := f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 3})
	over.Rules.MaxCardDuplicates = 2

	_, err := presetdeck.Decide(f.register(t), atMax, changedAt)
	require.NoError(t, err)
	_, err = presetdeck.Decide(f.register(t), over, changedAt)

	require.ErrorIs(t, err, presetdeck.ErrCardCountOutOfRange)
}

// 0 にするとデッキから外れる（枚数が 0 のカードは持たない）
func TestEvolve_CardCountsChangedEventSetsAndRemovesCards(t *testing.T) {
	f := newFixture(t)
	state := f.setCounts(t, f.register(t), map[presetdeck.CardRef]int{f.dragon: 3, f.potion: 2})
	state = f.setCounts(t, state, map[presetdeck.CardRef]int{f.dragon: 0, f.potion: 4})

	deck := state.(presetdeck.PresetDeck)

	require.Equal(t, map[presetdeck.CardRef]presetdeck.CardCount{f.potion: presetdecktest.CardCount(t, 4)}, deck.Cards)
	require.Equal(t, 4, deck.TotalCount())
	require.Equal(t, int64(3), deck.Version)
}

// 保存済みのイベントが壊れていても、範囲の外の枚数の状態を作らない
func TestEvolve_RejectsDeltaOutOfRange(t *testing.T) {
	f := newFixture(t)
	event := &presetdeck.CardCountsChangedEvent{
		EventMeta: presetdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Changes:   []presetdeck.CardCountChange{{Card: f.dragon, Delta: -1}},
		ChangedBy: f.operatorID,
	}

	_, err := presetdeck.Evolve(f.register(t), event)

	require.ErrorIs(t, err, presetdeck.ErrCardCountOutOfRange)
}

func TestEvolve_DoesNotChangeTheGivenState(t *testing.T) {
	f := newFixture(t)
	before := f.setCounts(t, f.register(t), map[presetdeck.CardRef]int{f.dragon: 3})

	f.setCounts(t, before, map[presetdeck.CardRef]int{f.dragon: 1})

	require.Equal(t, presetdecktest.CardCount(t, 3), before.(presetdeck.PresetDeck).Cards[f.dragon])
}

func TestDecide_DeleteMakesDeletedPresetDeck(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.register(t), &presetdeck.DeleteCommand{DeletedBy: f.operatorID}, changedAt)

	require.Equal(t, []presetdeck.Event{&presetdeck.DeletedEvent{
		EventMeta: presetdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		DeletedBy: f.operatorID,
	}}, events)
	require.Equal(t, presetdeck.DeletedPresetDeck{ID: f.id, Version: 2}, state)
}

func TestDecide_RejectsChangesUnlessRegistered(t *testing.T) {
	f := newFixture(t)
	deleted, _ := apply(t, f.register(t), &presetdeck.DeleteCommand{DeletedBy: f.operatorID}, changedAt)
	commands := []struct {
		name string
		cmd  presetdeck.Command
	}{
		{name: "rename", cmd: &presetdeck.RenameCommand{Name: presetdecktest.Name(t, "水のデッキ"), RenamedBy: f.operatorID}},
		{name: "set_card_counts", cmd: f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 1})},
		{name: "delete", cmd: &presetdeck.DeleteCommand{DeletedBy: f.operatorID}},
	}
	states := []struct {
		name  string
		state presetdeck.State
	}{
		{name: "unregistered", state: presetdeck.InitialState()},
		{name: "deleted", state: deleted},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := presetdeck.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, presetdeck.ErrInvalidState)
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
		{name: "blank", value: " ", want: presetdeck.ErrNameEmpty},
		{name: "too_long", value: strings.Repeat("あ", presetdeck.MaxNameLength+1), want: presetdeck.ErrNameTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := presetdeck.NewName(tt.value)

			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestNewCardCount_RejectsNegative(t *testing.T) {
	tests := []struct {
		name  string
		value int
		ok    bool
	}{
		{name: "negative", value: -1, ok: false},
		{name: "zero", value: 0, ok: true},
		{name: "positive", value: 4, ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := presetdeck.NewCardCount(tt.value)

			if tt.ok {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, presetdeck.ErrCardCountOutOfRange)
		})
	}
}

func TestNewCardRef_RejectsAggregateThatIsNotCard(t *testing.T) {
	_, err := presetdeck.NewCardRef(operator.Types().EntityType, 1)

	require.ErrorIs(t, err, presetdeck.ErrNotCard)
}

// 比べ方はカード名と同じ（全角・半角、大文字・小文字の違いを同じとみなす）
func TestNameKey_TreatsWidthAndCaseAsSame(t *testing.T) {
	require.Equal(t,
		presetdeck.NameKey(presetdecktest.Name(t, "Ｆire デッキ")),
		presetdeck.NameKey(presetdecktest.Name(t, "fire デッキ")))
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	f := newFixture(t)
	meta := presetdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt}
	events := []presetdeck.Event{
		&presetdeck.RegisteredEvent{EventMeta: meta, PublicID: publicID, Name: f.name, RegisteredBy: f.operatorID},
		&presetdeck.RenamedEvent{EventMeta: meta, Name: presetdecktest.Name(t, "水のデッキ"), RenamedBy: f.operatorID},
		&presetdeck.CardCountsChangedEvent{EventMeta: meta, Changes: []presetdeck.CardCountChange{
			{Card: f.dragon, Delta: -2},
			{Card: f.potion, Delta: 3},
		}, ChangedBy: f.operatorID},
		&presetdeck.DeletedEvent{EventMeta: meta, DeletedBy: f.operatorID},
	}
	for _, event := range events {
		t.Run(event.EventType().String(), func(t *testing.T) {
			require.Equal(t, 1, event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := presetdeck.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, event, restored)
		})
	}
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := presetdeck.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(presetdeck.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}
