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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck/npcdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck/presetdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

var (
	npcDeckRegisteredAt = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	npcDeckChangedAt    = time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
)

// 集約の Decide・Evolve でコマンドを順に当て、そのイベントを read model に当てる
type npcDeckHistory struct {
	t      *testing.T
	state  npcdeck.State
	events []npcdeck.Event
}

func newNPCDeckHistory(t *testing.T, publicID uuid.UUID, name string) *npcDeckHistory {
	t.Helper()
	return newNPCDeckHistoryFrom(t, &npcdeck.RegisterCommand{
		ID: npcdecktest.ID(t, 1), PublicID: publicID, Name: presetdecktest.Name(t, name), RegisteredBy: operatortest.ID(t, 10),
	})
}

func newNPCDeckHistoryFrom(t *testing.T, register *npcdeck.RegisterCommand) *npcDeckHistory {
	t.Helper()
	h := &npcDeckHistory{t: t, state: npcdeck.InitialState()}
	h.apply(register, npcDeckRegisteredAt)
	return h
}

func (h *npcDeckHistory) apply(cmd npcdeck.Command, at time.Time) {
	h.t.Helper()
	events, err := npcdeck.Decide(h.state, cmd, at)
	require.NoError(h.t, err)
	for _, event := range events {
		h.state, err = npcdeck.Evolve(h.state, event)
		require.NoError(h.t, err)
	}
	h.events = append(h.events, events...)
}

func (h *npcDeckHistory) setCounts(counts map[card.Ref]int) {
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
	h.apply(&npcdeck.SetCardCountsCommand{Set: set, Remove: remove, Rules: gamerules.Defaults().Deck, ChangedBy: operatortest.ID(h.t, 10)}, npcDeckChangedAt)
}

func (h *npcDeckHistory) project() readmodel.NPCDeck {
	var state readmodel.NPCDeck
	for _, event := range h.events {
		state = state.Evolve(event)
	}
	return state
}

func TestNPCDeck_EvolvesFromRegisteredWithoutSource(t *testing.T) {
	publicID := uuid.New()

	state := newNPCDeckHistory(t, publicID, "Ｆire デッキ").project()

	metadata := readmodel.NPCDeckMetadata{
		ID: publicID, Name: "Ｆire デッキ", Status: readmodel.NPCDeckStatusDraft, Revision: 1,
		RegisteredAt: npcDeckRegisteredAt, UpdatedAt: npcDeckRegisteredAt,
	}
	require.Equal(t, readmodel.NPCDeck{
		NPCDeckID: publicID,
		// 並べ替え・検索は正規化した名前、表示は入力どおりの名前
		SortName:     "fire デッキ",
		Status:       readmodel.NPCDeckStatusDraft,
		Metadata:     metadata,
		RegisteredBy: operatortest.ID(t, 10),
		UpdatedBy:    operatortest.ID(t, 10),
		Cards:        map[card.Ref]int{},
	}, state)
	require.False(t, state.IsTerminal())
}

// 写して作ったときは、カードを持つ。写した元は登録のイベントにだけ記録し、read model には持たせない
func TestNPCDeck_EvolvesFromRegisteredWithCopiedCards(t *testing.T) {
	publicID := uuid.New()
	dragon := cardtest.Ref(t, creature.Types().EntityType, 20)
	potion := cardtest.Ref(t, item.Types().EntityType, 21)
	source := npcdecktest.Source(t, presetdecktest.ID(t, 40), 4)

	state := newNPCDeckHistoryFrom(t, &npcdeck.RegisterCommand{
		ID: npcdecktest.ID(t, 1), PublicID: publicID, Name: presetdecktest.Name(t, "炎のデッキ"), Source: &source,
		Cards:        map[card.Ref]card.DeckQuantity{dragon: cardtest.DeckQuantity(t, 3), potion: cardtest.DeckQuantity(t, 2)},
		RegisteredBy: operatortest.ID(t, 10),
	}).project()

	require.Equal(t, map[card.Ref]int{dragon: 3, potion: 2}, state.Cards)
	require.Equal(t, 5, state.TotalCount())
}

func TestNPCDeck_EvolvesFromRenamed(t *testing.T) {
	history := newNPCDeckHistory(t, uuid.New(), "炎のデッキ")
	history.apply(&npcdeck.RenameCommand{Name: presetdecktest.Name(t, "Water デッキ"), RenamedBy: operatortest.ID(t, 11)}, npcDeckChangedAt)

	state := history.project()

	require.Equal(t, "water デッキ", state.SortName)
	require.Equal(t, "Water デッキ", state.Metadata.Name)
	require.Equal(t, int64(2), state.Metadata.Revision)
	require.Equal(t, npcDeckRegisteredAt, state.Metadata.RegisteredAt)
	require.Equal(t, npcDeckChangedAt, state.Metadata.UpdatedAt)
	require.Equal(t, operatortest.ID(t, 10), state.RegisteredBy)
	require.Equal(t, operatortest.ID(t, 11), state.UpdatedBy)
}

// 差分を足して枚数を作り、0 になったカードは持たない
func TestNPCDeck_EvolvesFromCardCountsChanged(t *testing.T) {
	dragon := cardtest.Ref(t, creature.Types().EntityType, 20)
	potion := cardtest.Ref(t, item.Types().EntityType, 21)
	history := newNPCDeckHistory(t, uuid.New(), "炎のデッキ")
	history.setCounts(map[card.Ref]int{dragon: 3, potion: 2})
	history.setCounts(map[card.Ref]int{dragon: 0, potion: 4})

	state := history.project()

	require.Equal(t, map[card.Ref]int{potion: 4}, state.Cards)
	require.Equal(t, 4, state.TotalCount())
	require.Equal(t, int64(3), state.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 10), state.UpdatedBy)
}

// 状態は絞り込みの列と、表示の metadata の両方に持つ
func TestNPCDeck_EvolvesStatusFromPublishedAndRetired(t *testing.T) {
	dragon := cardtest.Ref(t, creature.Types().EntityType, 20)
	history := newNPCDeckHistory(t, uuid.New(), "炎のデッキ")
	rules := gamerules.Defaults().Deck
	rules.TotalCardCount = 4
	history.apply(&npcdeck.SetCardCountsCommand{
		Set:   map[card.Ref]card.DeckQuantity{dragon: cardtest.DeckQuantity(t, 4)},
		Rules: rules, ChangedBy: operatortest.ID(t, 10),
	}, npcDeckChangedAt)
	history.apply(&npcdeck.PublishCommand{Rules: rules, PublishedBy: operatortest.ID(t, 11)}, npcDeckChangedAt)
	published := history.project()
	history.apply(&npcdeck.RetireCommand{RetiredBy: operatortest.ID(t, 12)}, npcDeckChangedAt)
	retired := history.project()

	require.Equal(t, readmodel.NPCDeckStatusPublished, published.Status)
	require.Equal(t, readmodel.NPCDeckStatusPublished, published.Metadata.Status)
	require.Equal(t, int64(3), published.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 11), published.UpdatedBy)
	require.Equal(t, readmodel.NPCDeckStatusRetired, retired.Status)
	require.Equal(t, readmodel.NPCDeckStatusRetired, retired.Metadata.Status)
	require.Equal(t, int64(4), retired.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 12), retired.UpdatedBy)
}

func TestNPCDeck_DoesNotChangeTheGivenState(t *testing.T) {
	dragon := cardtest.Ref(t, creature.Types().EntityType, 20)
	history := newNPCDeckHistory(t, uuid.New(), "炎のデッキ")
	history.setCounts(map[card.Ref]int{dragon: 3})
	before := history.project()
	history.setCounts(map[card.Ref]int{dragon: 1})

	before.Evolve(history.events[len(history.events)-1])

	require.Equal(t, map[card.Ref]int{dragon: 3}, before.Cards)
}

func TestNPCDeck_BecomesTerminalWhenDeleted(t *testing.T) {
	history := newNPCDeckHistory(t, uuid.New(), "炎のデッキ")
	history.apply(&npcdeck.DeleteCommand{DeletedBy: operatortest.ID(t, 10)}, npcDeckChangedAt)

	require.True(t, history.project().IsTerminal())
}

func TestNPCDeck_IgnoresOtherAggregateEvents(t *testing.T) {
	state := newNPCDeckHistory(t, uuid.New(), "炎のデッキ").project()

	evolved := state
	for _, event := range registeredCreature(t, uuid.New(), "ドラゴン", npcDeckChangedAt) {
		evolved = evolved.Evolve(event)
	}

	require.Equal(t, state, evolved)
}

func TestNewNPCDeckStatus_RejectsUnknownValue(t *testing.T) {
	_, err := readmodel.NewNPCDeckStatus("archived")

	require.Error(t, err)
}
