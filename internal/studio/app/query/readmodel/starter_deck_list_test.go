package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck/starterdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query/readmodel"
)

var (
	deckRegisteredAt = time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	deckChangedAt    = time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
)

// 集約の Decide・Evolve でコマンドを順に当て、そのイベントを read model に当てる
type starterDeckHistory struct {
	t      *testing.T
	state  starterdeck.State
	events []starterdeck.Event
}

func newStarterDeckHistory(t *testing.T, publicID uuid.UUID, name string) *starterDeckHistory {
	t.Helper()
	h := &starterDeckHistory{t: t, state: starterdeck.InitialState()}
	h.apply(&starterdeck.RegisterCommand{
		ID: starterdecktest.ID(t, 1), PublicID: publicID, Name: starterdecktest.Name(t, name), RegisteredBy: operatortest.ID(t, 10),
	}, deckRegisteredAt)
	return h
}

func (h *starterDeckHistory) apply(cmd starterdeck.Command, at time.Time) {
	h.t.Helper()
	events, err := starterdeck.Decide(h.state, cmd, at)
	require.NoError(h.t, err)
	for _, event := range events {
		h.state, err = starterdeck.Evolve(h.state, event)
		require.NoError(h.t, err)
	}
	h.events = append(h.events, events...)
}

func (h *starterDeckHistory) setCounts(counts map[starterdeck.CardRef]int) {
	h.t.Helper()
	cardCounts := make(map[starterdeck.CardRef]starterdeck.CardCount, len(counts))
	for card, count := range counts {
		cardCounts[card] = starterdecktest.CardCount(h.t, count)
	}
	h.apply(&starterdeck.SetCardCountsCommand{Counts: cardCounts, ChangedBy: operatortest.ID(h.t, 10)}, deckChangedAt)
}

func (h *starterDeckHistory) project() readmodel.StarterDeckList {
	var state readmodel.StarterDeckList
	for _, event := range h.events {
		state = state.Evolve(event)
	}
	return state
}

func TestStarterDeckList_EvolvesFromRegistered(t *testing.T) {
	publicID := uuid.New()

	state := newStarterDeckHistory(t, publicID, "Ｆire デッキ").project()

	metadata := readmodel.StarterDeckListMetadata{
		ID:           publicID,
		Name:         "Ｆire デッキ",
		Revision:     1,
		RegisteredAt: deckRegisteredAt,
		UpdatedAt:    deckRegisteredAt,
	}
	require.Equal(t, readmodel.StarterDeckList{
		StarterDeckID: publicID,
		// 並べ替え・検索は正規化した名前、表示は入力どおりの名前
		SortName: "fire デッキ",
		Metadata: metadata,
		Cards:    map[starterdeck.CardRef]int{},
	}, state)
	require.False(t, state.IsTerminal())
}

func TestStarterDeckList_EvolvesFromRenamed(t *testing.T) {
	history := newStarterDeckHistory(t, uuid.New(), "炎のデッキ")
	history.apply(&starterdeck.RenameCommand{Name: starterdecktest.Name(t, "Water デッキ"), RenamedBy: operatortest.ID(t, 10)}, deckChangedAt)

	state := history.project()

	require.Equal(t, "water デッキ", state.SortName)
	require.Equal(t, "Water デッキ", state.Metadata.Name)
	require.Equal(t, int64(2), state.Metadata.Revision)
	require.Equal(t, deckRegisteredAt, state.Metadata.RegisteredAt)
	require.Equal(t, deckChangedAt, state.Metadata.UpdatedAt)
}

// 差分を足して枚数を作り、0 になったカードは持たない
func TestStarterDeckList_EvolvesFromCardCountsChanged(t *testing.T) {
	dragon := starterdecktest.CardRef(t, creature.Types().EntityType, 20)
	potion := starterdecktest.CardRef(t, item.Types().EntityType, 21)
	history := newStarterDeckHistory(t, uuid.New(), "炎のデッキ")
	history.setCounts(map[starterdeck.CardRef]int{dragon: 3, potion: 2})
	history.setCounts(map[starterdeck.CardRef]int{dragon: 0, potion: 4})

	state := history.project()

	require.Equal(t, map[starterdeck.CardRef]int{potion: 4}, state.Cards)
	require.Equal(t, 4, state.TotalCount())
	require.Equal(t, int64(3), state.Metadata.Revision)
	require.Equal(t, deckChangedAt, state.Metadata.UpdatedAt)
}

func TestStarterDeckList_DoesNotChangeTheGivenState(t *testing.T) {
	dragon := starterdecktest.CardRef(t, creature.Types().EntityType, 20)
	history := newStarterDeckHistory(t, uuid.New(), "炎のデッキ")
	history.setCounts(map[starterdeck.CardRef]int{dragon: 3})
	before := history.project()
	history.setCounts(map[starterdeck.CardRef]int{dragon: 1})

	before.Evolve(history.events[len(history.events)-1])

	require.Equal(t, map[starterdeck.CardRef]int{dragon: 3}, before.Cards)
}

func TestStarterDeckList_BecomesTerminalWhenDeleted(t *testing.T) {
	history := newStarterDeckHistory(t, uuid.New(), "炎のデッキ")
	history.apply(&starterdeck.DeleteCommand{DeletedBy: operatortest.ID(t, 10)}, deckChangedAt)

	require.True(t, history.project().IsTerminal())
}

// カードの一覧の投影と同じく、購読していない集約のイベントは状態を変えない
func TestStarterDeckList_IgnoresOtherAggregateEvents(t *testing.T) {
	state := newStarterDeckHistory(t, uuid.New(), "炎のデッキ").project()

	evolved := state
	for _, event := range registeredCreature(t, uuid.New(), "ドラゴン", deckChangedAt) {
		evolved = evolved.Evolve(event)
	}

	require.Equal(t, state, evolved)
}
