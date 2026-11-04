package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck/presetdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
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

func (h *presetDeckHistory) setCounts(counts map[card.Ref]int) {
	h.t.Helper()
	set := make(map[card.Ref]card.DeckQuantity, len(counts))
	var remove []card.Ref
	for ref, count := range counts {
		if count == 0 {
			remove = append(remove, ref)
			continue
		}
		set[ref] = cardtest.DeckQuantity(h.t, count)
	}
	h.apply(&presetdeck.SetCardCountsCommand{Set: set, Remove: remove, Rules: gamerules.Defaults().Deck, ChangedBy: operatortest.ID(h.t, 12)}, deckChangedAt)
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
		Cards:    map[card.Ref]int{},
		// 登録の直後は、更新した人も登録した人
		RegisteredBy: operatortest.ID(t, 10), UpdatedBy: operatortest.ID(t, 10),
	}, state)
	require.False(t, state.IsTerminal())
}

func TestPresetDeck_EvolvesFromRenamed(t *testing.T) {
	history := newPresetDeckHistory(t, uuid.New(), "炎のデッキ")
	history.apply(&presetdeck.RenameCommand{Name: presetdecktest.Name(t, "Water デッキ"), RenamedBy: operatortest.ID(t, 11)}, deckChangedAt)

	state := history.project()

	require.Equal(t, "water デッキ", state.SortName)
	require.Equal(t, "Water デッキ", state.Metadata.Name)
	require.Equal(t, int64(2), state.Metadata.Revision)
	require.Equal(t, deckRegisteredAt, state.Metadata.RegisteredAt)
	require.Equal(t, deckChangedAt, state.Metadata.UpdatedAt)
	require.Equal(t, operatortest.ID(t, 10), state.RegisteredBy)
	require.Equal(t, operatortest.ID(t, 11), state.UpdatedBy)
}

// 差分を足して枚数を作り、0 になったカードは持たない
func TestPresetDeck_EvolvesFromCardCountsChanged(t *testing.T) {
	dragon := cardtest.Ref(t, creature.Types().EntityType, 20)
	potion := cardtest.Ref(t, item.Types().EntityType, 21)
	history := newPresetDeckHistory(t, uuid.New(), "炎のデッキ")
	history.setCounts(map[card.Ref]int{dragon: 3, potion: 2})
	history.setCounts(map[card.Ref]int{dragon: 0, potion: 4})

	state := history.project()

	require.Equal(t, map[card.Ref]int{potion: 4}, state.Cards)
	require.Equal(t, 4, state.TotalCount())
	require.Equal(t, int64(3), state.Metadata.Revision)
	require.Equal(t, deckChangedAt, state.Metadata.UpdatedAt)
	require.Equal(t, operatortest.ID(t, 10), state.RegisteredBy)
	require.Equal(t, operatortest.ID(t, 12), state.UpdatedBy)
}

func TestPresetDeck_DoesNotChangeTheGivenState(t *testing.T) {
	dragon := cardtest.Ref(t, creature.Types().EntityType, 20)
	history := newPresetDeckHistory(t, uuid.New(), "炎のデッキ")
	history.setCounts(map[card.Ref]int{dragon: 3})
	before := history.project()
	history.setCounts(map[card.Ref]int{dragon: 1})

	before.Evolve(history.events[len(history.events)-1])

	require.Equal(t, map[card.Ref]int{dragon: 3}, before.Cards)
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
