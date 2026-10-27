package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules/studiorulestest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

func TestStudioRules_EvolvesFromCreated(t *testing.T) {
	publicID := uuid.New()
	at := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	rules := studiorules.Defaults()
	event := &studiorules.CreatedEvent{
		EventMeta: studiorules.EventMeta{EntityID: studiorulestest.ID(t, 1), AggregateVersion: 1, OccurredAt: at},
		PublicID:  publicID,
		Rules:     rules,
	}

	state := readmodel.StudioRules{}.Evolve(event)

	require.Equal(t, readmodel.StudioRules{StudioRulesID: publicID, Revision: 1, UpdatedAt: at, Rules: rules}, state)
	require.False(t, state.IsTerminal())
}

// 購読していない集約のイベントは状態を変えない
func TestStudioRules_IgnoresOtherAggregateEvents(t *testing.T) {
	state := readmodel.StudioRules{StudioRulesID: uuid.New(), Revision: 1}

	evolved := state
	for _, event := range registeredCreature(t, uuid.New(), "ドラゴン", time.Now().UTC()) {
		evolved = evolved.Evolve(event)
	}

	require.Equal(t, state, evolved)
}
