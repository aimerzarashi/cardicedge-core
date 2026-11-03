package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck/grantdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
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

func (h *grantDeckHistory) setCounts(counts map[presetdeck.CardRef]int) {
	h.t.Helper()
	cardCounts := make(map[presetdeck.CardRef]presetdeck.CardCount, len(counts))
	for card, count := range counts {
		cardCounts[card] = presetdecktest.CardCount(h.t, count)
	}
	h.apply(&grantdeck.SetCardCountsCommand{Counts: cardCounts, Rules: gamerules.Defaults().Deck, ChangedBy: operatortest.ID(h.t, 12)}, deckChangedAt)
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
		Cards:    map[presetdeck.CardRef]int{},
		// 登録の直後は、更新した人も登録した人
		RegisteredBy: operatortest.ID(t, 10), UpdatedBy: operatortest.ID(t, 10),
	}, state)
	require.False(t, state.IsTerminal())
}

// 写して作ったときは、カードと写した元（公開 ID と revision）を持つ
func TestGrantDeck_EvolvesFromRegisteredWithSourceAndCards(t *testing.T) {
	publicID, presetID := uuid.New(), uuid.New()
	dragon := presetdecktest.CardRef(t, creature.Types().EntityType, 20)
	potion := presetdecktest.CardRef(t, item.Types().EntityType, 21)
	source := grantdecktest.Source(t, presetID, 4)

	state := newGrantDeckHistoryFrom(t, &grantdeck.RegisterCommand{
		ID: grantdecktest.ID(t, 1), PublicID: publicID, Name: presetdecktest.Name(t, "炎のデッキ"), Source: &source,
		Cards:        map[presetdeck.CardRef]presetdeck.CardCount{dragon: presetdecktest.CardCount(t, 3), potion: presetdecktest.CardCount(t, 2)},
		RegisteredBy: operatortest.ID(t, 10),
	}).project()

	require.Equal(t, map[presetdeck.CardRef]int{dragon: 3, potion: 2}, state.Cards)
	require.Equal(t, 5, state.TotalCount())
	require.Equal(t, &readmodel.GrantDeckSource{PresetDeckID: presetID, Revision: 4}, state.Metadata.Source)
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
	dragon := presetdecktest.CardRef(t, creature.Types().EntityType, 20)
	potion := presetdecktest.CardRef(t, item.Types().EntityType, 21)
	history := newGrantDeckHistory(t, uuid.New(), "炎のデッキ")
	history.setCounts(map[presetdeck.CardRef]int{dragon: 3, potion: 2})
	history.setCounts(map[presetdeck.CardRef]int{dragon: 0, potion: 4})

	state := history.project()

	require.Equal(t, map[presetdeck.CardRef]int{potion: 4}, state.Cards)
	require.Equal(t, 4, state.TotalCount())
	require.Equal(t, int64(3), state.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 10), state.RegisteredBy)
	require.Equal(t, operatortest.ID(t, 12), state.UpdatedBy)
}

// 状態は絞り込みの列と、表示の metadata の両方に持つ
func TestGrantDeck_EvolvesStatusFromPublishedAndRetired(t *testing.T) {
	dragon := presetdecktest.CardRef(t, creature.Types().EntityType, 20)
	history := newGrantDeckHistory(t, uuid.New(), "炎のデッキ")
	rules := gamerules.Defaults().Deck
	rules.TotalCardCount = 4
	history.apply(&grantdeck.SetCardCountsCommand{
		Counts: map[presetdeck.CardRef]presetdeck.CardCount{dragon: presetdecktest.CardCount(t, 4)},
		Rules:  rules, ChangedBy: operatortest.ID(t, 10),
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
	dragon := presetdecktest.CardRef(t, creature.Types().EntityType, 20)
	history := newGrantDeckHistory(t, uuid.New(), "炎のデッキ")
	history.setCounts(map[presetdeck.CardRef]int{dragon: 3})
	before := history.project()
	history.setCounts(map[presetdeck.CardRef]int{dragon: 1})

	before.Evolve(history.events[len(history.events)-1])

	require.Equal(t, map[presetdeck.CardRef]int{dragon: 3}, before.Cards)
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
