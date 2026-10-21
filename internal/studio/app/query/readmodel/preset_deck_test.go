package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck/presetdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query/readmodel"
)

var (
	deckRegisteredAt = time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	deckChangedAt    = time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
)

// 集約の Decide・Evolve でコマンドを順に当て、そのイベントを read model に当てる
type presetDeckHistory struct {
	t      *testing.T
	state  presetdeck.State
	events []presetdeck.Event
}

func newPresetDeckHistory(t *testing.T, publicID uuid.UUID, name string) *presetDeckHistory {
	t.Helper()
	h := &presetDeckHistory{t: t, state: presetdeck.InitialState()}
	h.apply(&presetdeck.RegisterCommand{
		ID: presetdecktest.ID(t, 1), PublicID: publicID, Name: presetdecktest.Name(t, name), RegisteredBy: operatortest.ID(t, 10),
	}, deckRegisteredAt)
	return h
}

func (h *presetDeckHistory) apply(cmd presetdeck.Command, at time.Time) {
	h.t.Helper()
	events, err := presetdeck.Decide(h.state, cmd, at)
	require.NoError(h.t, err)
	for _, event := range events {
		h.state, err = presetdeck.Evolve(h.state, event)
		require.NoError(h.t, err)
	}
	h.events = append(h.events, events...)
}

func (h *presetDeckHistory) setCounts(counts map[presetdeck.CardRef]int) {
	h.t.Helper()
	cardCounts := make(map[presetdeck.CardRef]presetdeck.CardCount, len(counts))
	for card, count := range counts {
		cardCounts[card] = presetdecktest.CardCount(h.t, count)
	}
	h.apply(&presetdeck.SetCardCountsCommand{Counts: cardCounts, ChangedBy: operatortest.ID(h.t, 10)}, deckChangedAt)
}

func (h *presetDeckHistory) project() readmodel.PresetDeck {
	var state readmodel.PresetDeck
	for _, event := range h.events {
		state = state.Evolve(event)
	}
	return state
}

func TestPresetDeck_EvolvesFromRegistered(t *testing.T) {
	publicID := uuid.New()

	state := newPresetDeckHistory(t, publicID, "Ｆire デッキ").project()

	metadata := readmodel.PresetDeckMetadata{
		ID:           publicID,
		Name:         "Ｆire デッキ",
		Revision:     1,
		RegisteredAt: deckRegisteredAt,
		UpdatedAt:    deckRegisteredAt,
	}
	require.Equal(t, readmodel.PresetDeck{
		PresetDeckID: publicID,
		// 並べ替え・検索は正規化した名前、表示は入力どおりの名前
		SortName: "fire デッキ",
		Metadata: metadata,
		Cards:    map[presetdeck.CardRef]int{},
	}, state)
	require.False(t, state.IsTerminal())
}

func TestPresetDeck_EvolvesFromRenamed(t *testing.T) {
	history := newPresetDeckHistory(t, uuid.New(), "炎のデッキ")
	history.apply(&presetdeck.RenameCommand{Name: presetdecktest.Name(t, "Water デッキ"), RenamedBy: operatortest.ID(t, 10)}, deckChangedAt)

	state := history.project()

	require.Equal(t, "water デッキ", state.SortName)
	require.Equal(t, "Water デッキ", state.Metadata.Name)
	require.Equal(t, int64(2), state.Metadata.Revision)
	require.Equal(t, deckRegisteredAt, state.Metadata.RegisteredAt)
	require.Equal(t, deckChangedAt, state.Metadata.UpdatedAt)
}

// 差分を足して枚数を作り、0 になったカードは持たない
func TestPresetDeck_EvolvesFromCardCountsChanged(t *testing.T) {
	dragon := presetdecktest.CardRef(t, creature.Types().EntityType, 20)
	potion := presetdecktest.CardRef(t, item.Types().EntityType, 21)
	history := newPresetDeckHistory(t, uuid.New(), "炎のデッキ")
	history.setCounts(map[presetdeck.CardRef]int{dragon: 3, potion: 2})
	history.setCounts(map[presetdeck.CardRef]int{dragon: 0, potion: 4})

	state := history.project()

	require.Equal(t, map[presetdeck.CardRef]int{potion: 4}, state.Cards)
	require.Equal(t, 4, state.TotalCount())
	require.Equal(t, int64(3), state.Metadata.Revision)
	require.Equal(t, deckChangedAt, state.Metadata.UpdatedAt)
}

func TestPresetDeck_DoesNotChangeTheGivenState(t *testing.T) {
	dragon := presetdecktest.CardRef(t, creature.Types().EntityType, 20)
	history := newPresetDeckHistory(t, uuid.New(), "炎のデッキ")
	history.setCounts(map[presetdeck.CardRef]int{dragon: 3})
	before := history.project()
	history.setCounts(map[presetdeck.CardRef]int{dragon: 1})

	before.Evolve(history.events[len(history.events)-1])

	require.Equal(t, map[presetdeck.CardRef]int{dragon: 3}, before.Cards)
}

func TestPresetDeck_BecomesTerminalWhenDeleted(t *testing.T) {
	history := newPresetDeckHistory(t, uuid.New(), "炎のデッキ")
	history.apply(&presetdeck.DeleteCommand{DeletedBy: operatortest.ID(t, 10)}, deckChangedAt)

	require.True(t, history.project().IsTerminal())
}

// カードの一覧の投影と同じく、購読していない集約のイベントは状態を変えない
func TestPresetDeck_IgnoresOtherAggregateEvents(t *testing.T) {
	state := newPresetDeckHistory(t, uuid.New(), "炎のデッキ").project()

	evolved := state
	for _, event := range registeredCreature(t, uuid.New(), "ドラゴン", deckChangedAt) {
		evolved = evolved.Evolve(event)
	}

	require.Equal(t, state, evolved)
}
