package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules/gamerulestest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

func TestGameRules_EvolvesFromCreated(t *testing.T) {
	publicID := uuid.New()
	at := time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	rules := gamerules.Defaults()
	event := &gamerules.CreatedEvent{
		EventMeta: gamerules.EventMeta{EntityID: gamerulestest.ID(t, 1), AggregateVersion: 1, OccurredAt: at},
		PublicID:  publicID,
		Rules:     rules,
	}

	state := readmodel.GameRules{}.Evolve(event)

	require.Equal(t, readmodel.GameRules{GameRulesID: publicID, Revision: 1, UpdatedAt: at, Rules: rules}, state)
	require.False(t, state.IsTerminal())
}

// 購読していない集約のイベントは状態を変えない
func TestGameRules_IgnoresOtherAggregateEvents(t *testing.T) {
	state := readmodel.GameRules{GameRulesID: uuid.New(), Revision: 1}

	evolved := state
	for _, event := range registeredCreature(t, uuid.New(), "ドラゴン", time.Now().UTC()) {
		evolved = evolved.Evolve(event)
	}

	require.Equal(t, state, evolved)
}
