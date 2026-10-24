package readmodel_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

func TestPlayer_EvolvesFromRegistered(t *testing.T) {
	publicID := uuid.New()
	event := &player.RegisteredEvent{
		EventMeta: player.EventMeta{EntityID: playertest.ID(t, 1), AggregateVersion: 1},
		PublicID:  publicID,
		Issuer:    playertest.Issuer(t, "https://issuer.example"),
		Subject:   playertest.Subject(t, "sub"),
	}

	state := readmodel.Player{}.Evolve(event)

	require.Equal(t, readmodel.Player{PlayerID: publicID}, state)
	require.False(t, state.IsTerminal())
}

// 購読していない集約のイベントは状態を変えない
func TestPlayer_IgnoresOtherAggregateEvents(t *testing.T) {
	state := readmodel.Player{PlayerID: uuid.New()}

	evolved := state.Evolve(&startergrant.CompletedEvent{EventMeta: startergrant.EventMeta{EntityID: startergranttest.ID(t, 1), AggregateVersion: 2}})

	require.Equal(t, state, evolved)
}
