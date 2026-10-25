package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature/creaturetest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item/itemtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell/spelltest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

func TestCard_EvolvesFromCreatureRegistered(t *testing.T) {
	publicID := uuid.New()
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	events := registeredCreature(t, publicID, "Ｄragon", at)

	var state readmodel.Card
	for _, event := range events {
		state = state.Evolve(event)
	}

	metadata := readmodel.CardMetadata{ID: publicID, Name: "Ｄragon", Status: readmodel.CardStatusDraft, Revision: 1, RegisteredAt: at, UpdatedAt: at}
	require.Equal(t, readmodel.Card{
		CardID:   publicID,
		CardType: readmodel.CardTypeCreature,
		Status:   readmodel.CardStatusDraft,
		// 並べ替え・検索は正規化した名前、表示は入力どおりの名前
		SortName: "dragon",
		Metadata: metadata,
	}, state)
	require.False(t, state.IsTerminal())
}

func registeredItem(t *testing.T, publicID uuid.UUID, name string, at time.Time) []item.Event {
	t.Helper()
	events, err := item.Decide(item.InitialState(), &item.RegisterCommand{
		ID: itemtest.ID(t, 1), PublicID: publicID, Name: cardtest.Name(t, name), RegisteredBy: operatortest.ID(t, 10),
	}, at)
	require.NoError(t, err)
	return events
}

func TestCard_EvolvesFromItemRegistered(t *testing.T) {
	publicID := uuid.New()
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	events := registeredItem(t, publicID, "Ｐotion", at)

	var state readmodel.Card
	for _, event := range events {
		state = state.Evolve(event)
	}

	metadata := readmodel.CardMetadata{ID: publicID, Name: "Ｐotion", Status: readmodel.CardStatusDraft, Revision: 1, RegisteredAt: at, UpdatedAt: at}
	require.Equal(t, readmodel.Card{
		CardID:   publicID,
		CardType: readmodel.CardTypeItem,
		Status:   readmodel.CardStatusDraft,
		SortName: "potion",
		Metadata: metadata,
	}, state)
	require.False(t, state.IsTerminal())
}

func registeredSpell(t *testing.T, publicID uuid.UUID, name string, at time.Time) []spell.Event {
	t.Helper()
	events, err := spell.Decide(spell.InitialState(), &spell.RegisterCommand{
		ID: spelltest.ID(t, 1), PublicID: publicID, Name: cardtest.Name(t, name), RegisteredBy: operatortest.ID(t, 10),
	}, at)
	require.NoError(t, err)
	return events
}

func TestCard_EvolvesFromSpellRegistered(t *testing.T) {
	publicID := uuid.New()
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	events := registeredSpell(t, publicID, "Ｆireball", at)

	var state readmodel.Card
	for _, event := range events {
		state = state.Evolve(event)
	}

	metadata := readmodel.CardMetadata{ID: publicID, Name: "Ｆireball", Status: readmodel.CardStatusDraft, Revision: 1, RegisteredAt: at, UpdatedAt: at}
	require.Equal(t, readmodel.Card{
		CardID:   publicID,
		CardType: readmodel.CardTypeSpell,
		Status:   readmodel.CardStatusDraft,
		SortName: "fireball",
		Metadata: metadata,
	}, state)
	require.False(t, state.IsTerminal())
}

func TestCard_IgnoresEventsOfOtherAggregates(t *testing.T) {
	state := readmodel.Card{CardID: uuid.New(), CardType: readmodel.CardTypeCreature}

	evolved := state.Evolve(&operator.RegisteredEvent{})

	require.Equal(t, state, evolved)
}

// 公開・使用不可のイベントは、状態の列と metadata の状態・revision・最後に変更した日時を変える（登録した日時は変えない）
func TestCard_EvolvesStatusFromPublishedAndRetired(t *testing.T) {
	registeredAt := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	publishedAt := registeredAt.Add(time.Hour)
	retiredAt := publishedAt.Add(time.Hour)
	publicID := uuid.New()
	state := readmodel.Card{}.Evolve(registeredCreature(t, publicID, "Dragon", registeredAt)[0])
	registered := creature.Creature{ID: creaturetest.ID(t, 1), Version: 1, Status: card.StatusDraft}
	published, err := creature.Decide(registered, &creature.PublishCommand{PublishedBy: operatortest.ID(t, 10)}, publishedAt)
	require.NoError(t, err)
	publishedState, err := creature.Evolve(registered, published[0])
	require.NoError(t, err)
	retired, err := creature.Decide(publishedState, &creature.RetireCommand{RetiredBy: operatortest.ID(t, 10)}, retiredAt)
	require.NoError(t, err)

	afterPublished := state.Evolve(published[0])
	afterRetired := afterPublished.Evolve(retired[0])

	require.Equal(t, readmodel.CardStatusPublished, afterPublished.Status)
	require.Equal(t, readmodel.CardMetadata{
		ID: publicID, Name: "Dragon", Status: readmodel.CardStatusPublished, Revision: 2, RegisteredAt: registeredAt, UpdatedAt: publishedAt,
	}, afterPublished.Metadata)
	require.Equal(t, readmodel.CardStatusRetired, afterRetired.Status)
	require.Equal(t, readmodel.CardMetadata{
		ID: publicID, Name: "Dragon", Status: readmodel.CardStatusRetired, Revision: 3, RegisteredAt: registeredAt, UpdatedAt: retiredAt,
	}, afterRetired.Metadata)
	require.False(t, afterRetired.IsTerminal())
}

func TestNewCardStatus_RejectsUnknownValue(t *testing.T) {
	_, err := readmodel.NewCardStatus("archived")

	require.Error(t, err)
}
