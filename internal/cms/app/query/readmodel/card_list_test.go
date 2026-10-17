package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature/creaturetest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
)

func registeredCreature(t *testing.T, publicID uuid.UUID, name string, at time.Time) []creature.Event {
	t.Helper()
	events, err := creature.Decide(creature.InitialState(), &creature.RegisterCommand{
		ID: creaturetest.ID(t, 1), PublicID: publicID, Name: cardtest.Name(t, name), RegisteredBy: operatortest.ID(t, 10),
	}, at)
	require.NoError(t, err)
	return events
}

func TestCardList_EvolvesFromCreatureRegistered(t *testing.T) {
	publicID := uuid.New()
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	events := registeredCreature(t, publicID, "Ｄragon", at)

	var state readmodel.CardList
	for _, event := range events {
		state = state.Evolve(event)
	}

	metadata := readmodel.CardListMetadata{ID: publicID, Name: "Ｄragon", RegisteredAt: at, UpdatedAt: at}
	require.Equal(t, readmodel.CardList{
		CardID:   publicID,
		CardType: readmodel.CardTypeCreature,
		// 並べ替え・検索は正規化した名前、表示は入力どおりの名前
		SortName: "dragon",
		Metadata: metadata,
	}, state)
	require.False(t, state.IsTerminal())
}

func TestCardList_IgnoresEventsOfOtherAggregates(t *testing.T) {
	state := readmodel.CardList{CardID: uuid.New(), CardType: readmodel.CardTypeCreature}

	evolved := state.Evolve(&operator.RegisteredEvent{})

	require.Equal(t, state, evolved)
}
