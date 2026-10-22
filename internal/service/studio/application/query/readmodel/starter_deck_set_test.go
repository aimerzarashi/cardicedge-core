package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset/starterdecksettest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

func TestStarterDeckSet_EvolvesFromDecksSet(t *testing.T) {
	publicID := uuid.New()
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	event := &starterdeckset.DecksSetEvent{
		EventMeta: starterdeckset.EventMeta{EntityID: starterdecksettest.ID(t, 1), AggregateVersion: 3, OccurredAt: at},
		PublicID:  publicID,
		Decks:     []starterdeckset.Deck{},
		SetBy:     operatortest.ID(t, 10),
	}

	state := readmodel.StarterDeckSet{}.Evolve(event)

	require.Equal(t, readmodel.StarterDeckSet{StarterDeckSetID: publicID, Revision: 3, UpdatedAt: at, Decks: []starterdeckset.Deck{}}, state)
	require.False(t, state.IsTerminal())
}

// 購読していない集約のイベントは状態を変えない
func TestStarterDeckSet_IgnoresOtherAggregateEvents(t *testing.T) {
	state := readmodel.StarterDeckSet{StarterDeckSetID: uuid.New(), Revision: 1}

	evolved := state
	for _, event := range registeredCreature(t, uuid.New(), "ドラゴン", time.Now().UTC()) {
		evolved = evolved.Evolve(event)
	}

	require.Equal(t, state, evolved)
}
