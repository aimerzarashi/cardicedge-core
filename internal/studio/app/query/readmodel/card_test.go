package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature/creaturetest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item/itemtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell/spelltest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query/readmodel"
)

func registeredCreature(t *testing.T, publicID uuid.UUID, name string, at time.Time) []creature.Event {
	t.Helper()
	events, err := creature.Decide(creature.InitialState(), &creature.RegisterCommand{
		ID: creaturetest.ID(t, 1), PublicID: publicID, Name: cardtest.Name(t, name), RegisteredBy: operatortest.ID(t, 10),
	}, at)
	require.NoError(t, err)
	return events
}

func TestCard_EvolvesFromCreatureRegistered(t *testing.T) {
	publicID := uuid.New()
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	events := registeredCreature(t, publicID, "Ｄragon", at)

	var state readmodel.Card
	for _, event := range events {
		state = state.Evolve(event)
	}

	metadata := readmodel.CardMetadata{ID: publicID, Name: "Ｄragon", RegisteredAt: at, UpdatedAt: at}
	require.Equal(t, readmodel.Card{
		CardID:   publicID,
		CardType: readmodel.CardTypeCreature,
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

	metadata := readmodel.CardMetadata{ID: publicID, Name: "Ｐotion", RegisteredAt: at, UpdatedAt: at}
	require.Equal(t, readmodel.Card{
		CardID:   publicID,
		CardType: readmodel.CardTypeItem,
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

	metadata := readmodel.CardMetadata{ID: publicID, Name: "Ｆireball", RegisteredAt: at, UpdatedAt: at}
	require.Equal(t, readmodel.Card{
		CardID:   publicID,
		CardType: readmodel.CardTypeSpell,
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
