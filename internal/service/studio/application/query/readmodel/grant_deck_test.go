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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck/grantdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck/presetdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 集約の Decide・Evolve でコマンドを順に当て、そのイベントを read model に当てる
type grantDeckHistory struct {
	t      *testing.T
	state  grantdeck.State
	events []grantdeck.Event
}

func newGrantDeckHistory(t *testing.T, publicID uuid.UUID, name string) *grantDeckHistory {
	t.Helper()
	return newGrantDeckHistoryFrom(t, &grantdeck.RegisterCommand{
		ID: grantdecktest.ID(t, 1), PublicID: publicID, Name: presetdecktest.Name(t, name), RegisteredBy: operatortest.ID(t, 10),
	})
}

func newGrantDeckHistoryFrom(t *testing.T, register *grantdeck.RegisterCommand) *grantDeckHistory {
	t.Helper()
	h := &grantDeckHistory{t: t, state: grantdeck.InitialState()}
	h.apply(register, deckRegisteredAt)
	return h
}

func (h *grantDeckHistory) apply(cmd grantdeck.Command, at time.Time) {
	h.t.Helper()
	events, err := grantdeck.Decide(h.state, cmd, at)
	require.NoError(h.t, err)
	for _, event := range events {
		h.state, err = grantdeck.Evolve(h.state, event)
		require.NoError(h.t, err)
	}
	h.events = append(h.events, events...)
}

func (h *grantDeckHistory) setCounts(counts map[card.Ref]int) {
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
	h.apply(&grantdeck.SetCardCountsCommand{Set: set, Remove: remove, Rules: gamerules.Defaults().Deck, ChangedBy: operatortest.ID(h.t, 12)}, deckChangedAt)
}

func (h *grantDeckHistory) project() readmodel.GrantDeck {
	var state readmodel.GrantDeck
	for _, event := range h.events {
		state = state.Evolve(event)
	}
	return state
}

func TestGrantDeck_EvolvesFromRegisteredWithoutSource(t *testing.T) {
	publicID := uuid.New()

	state := newGrantDeckHistory(t, publicID, "Ｆire デッキ").project()

	metadata := readmodel.GrantDeckMetadata{
		ID: publicID, Name: "Ｆire デッキ", Status: readmodel.GrantDeckStatusDraft, Revision: 1,
		RegisteredAt: deckRegisteredAt, UpdatedAt: deckRegisteredAt,
	}
	require.Equal(t, readmodel.GrantDeck{
		GrantDeckID: publicID,
		// 並べ替え・検索は正規化した名前、表示は入力どおりの名前
		SortName: "fire デッキ",
		Status:   readmodel.GrantDeckStatusDraft,
		Metadata: metadata,
		Cards:    map[card.Ref]int{},
		// 登録の直後は、更新した人も登録した人
		RegisteredBy: operatortest.ID(t, 10), UpdatedBy: operatortest.ID(t, 10),
	}, state)
	require.False(t, state.IsTerminal())
}

// 写して作ったときは、カードを持つ。写した元は登録のイベントにだけ記録し、read model には持たせない
func TestGrantDeck_EvolvesFromRegisteredWithCopiedCards(t *testing.T) {
	publicID := uuid.New()
	dragon := cardtest.Ref(t, creature.Types().EntityType, 20)
	potion := cardtest.Ref(t, item.Types().EntityType, 21)
	source := grantdecktest.Source(t, presetdecktest.ID(t, 40), 4)

	state := newGrantDeckHistoryFrom(t, &grantdeck.RegisterCommand{
		ID: grantdecktest.ID(t, 1), PublicID: publicID, Name: presetdecktest.Name(t, "炎のデッキ"), Source: &source,
		Cards:        map[card.Ref]card.DeckQuantity{dragon: cardtest.DeckQuantity(t, 3), potion: cardtest.DeckQuantity(t, 2)},
		RegisteredBy: operatortest.ID(t, 10),
	}).project()

	require.Equal(t, map[card.Ref]int{dragon: 3, potion: 2}, state.Cards)
	require.Equal(t, 5, state.TotalCount())
}

func TestGrantDeck_EvolvesFromRenamed(t *testing.T) {
	history := newGrantDeckHistory(t, uuid.New(), "炎のデッキ")
	history.apply(&grantdeck.RenameCommand{Name: presetdecktest.Name(t, "Water デッキ"), RenamedBy: operatortest.ID(t, 11)}, deckChangedAt)

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
func TestGrantDeck_EvolvesFromCardCountsChanged(t *testing.T) {
	dragon := cardtest.Ref(t, creature.Types().EntityType, 20)
	potion := cardtest.Ref(t, item.Types().EntityType, 21)
	history := newGrantDeckHistory(t, uuid.New(), "炎のデッキ")
	history.setCounts(map[card.Ref]int{dragon: 3, potion: 2})
	history.setCounts(map[card.Ref]int{dragon: 0, potion: 4})

	state := history.project()

	require.Equal(t, map[card.Ref]int{potion: 4}, state.Cards)
	require.Equal(t, 4, state.TotalCount())
	require.Equal(t, int64(3), state.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 10), state.RegisteredBy)
	require.Equal(t, operatortest.ID(t, 12), state.UpdatedBy)
}

// 状態は絞り込みの列と、表示の metadata の両方に持つ
func TestGrantDeck_EvolvesStatusFromPublishedAndRetired(t *testing.T) {
	dragon := cardtest.Ref(t, creature.Types().EntityType, 20)
	history := newGrantDeckHistory(t, uuid.New(), "炎のデッキ")
	rules := gamerules.Defaults().Deck
	rules.TotalCardCount = cardtest.DeckTotalCount(t, 4)
	history.apply(&grantdeck.SetCardCountsCommand{
		Set:   map[card.Ref]card.DeckQuantity{dragon: cardtest.DeckQuantity(t, 4)},
		Rules: rules, ChangedBy: operatortest.ID(t, 10),
	}, deckChangedAt)
	history.apply(&grantdeck.PublishCommand{Rules: rules, PublishedBy: operatortest.ID(t, 11)}, deckChangedAt)
	published := history.project()
	history.apply(&grantdeck.RetireCommand{RetiredBy: operatortest.ID(t, 13)}, deckChangedAt)
	retired := history.project()

	require.Equal(t, readmodel.GrantDeckStatusPublished, published.Status)
	require.Equal(t, readmodel.GrantDeckStatusPublished, published.Metadata.Status)
	require.Equal(t, int64(3), published.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 11), published.UpdatedBy)
	require.Equal(t, readmodel.GrantDeckStatusRetired, retired.Status)
	require.Equal(t, readmodel.GrantDeckStatusRetired, retired.Metadata.Status)
	require.Equal(t, int64(4), retired.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 13), retired.UpdatedBy)
	require.Equal(t, operatortest.ID(t, 10), retired.RegisteredBy)
}

func TestGrantDeck_DoesNotChangeTheGivenState(t *testing.T) {
	dragon := cardtest.Ref(t, creature.Types().EntityType, 20)
	history := newGrantDeckHistory(t, uuid.New(), "炎のデッキ")
	history.setCounts(map[card.Ref]int{dragon: 3})
	before := history.project()
	history.setCounts(map[card.Ref]int{dragon: 1})

	before.Evolve(history.events[len(history.events)-1])

	require.Equal(t, map[card.Ref]int{dragon: 3}, before.Cards)
}

func TestGrantDeck_BecomesTerminalWhenDeleted(t *testing.T) {
	history := newGrantDeckHistory(t, uuid.New(), "炎のデッキ")
	history.apply(&grantdeck.DeleteCommand{DeletedBy: operatortest.ID(t, 10)}, deckChangedAt)

	require.True(t, history.project().IsTerminal())
}

func TestGrantDeck_IgnoresOtherAggregateEvents(t *testing.T) {
	state := newGrantDeckHistory(t, uuid.New(), "炎のデッキ").project()

	evolved := state
	for _, event := range registeredCreature(t, uuid.New(), "ドラゴン", deckChangedAt) {
		evolved = evolved.Evolve(event)
	}

	require.Equal(t, state, evolved)
}

func TestNewGrantDeckStatus_RejectsUnknownValue(t *testing.T) {
	_, err := readmodel.NewGrantDeckStatus("archived")

	require.Error(t, err)
}
