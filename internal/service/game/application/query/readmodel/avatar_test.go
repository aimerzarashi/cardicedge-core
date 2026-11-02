package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar/avatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

func TestAvatar_EvolvesFromRegistered(t *testing.T) {
	publicID := uuid.New()
	occurredAt := time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	event := &avatar.RegisteredEvent{
		EventMeta: avatar.EventMeta{EntityID: avatartest.ID(t, 1), AggregateVersion: 1, OccurredAt: occurredAt},
		PublicID:  publicID,
		Player:    playertest.ID(t, 7),
		Name:      avatartest.Name(t, "アリス"),
	}

	state := readmodel.Avatar{}.Evolve(event)

	require.Equal(t, readmodel.Avatar{AvatarID: publicID, Player: playertest.ID(t, 7), Name: "アリス", RegisteredAt: occurredAt}, state)
	require.Equal(t, readmodel.AvatarMetadata{ID: publicID, Name: "アリス", RegisteredAt: occurredAt, UpdatedAt: occurredAt}, state.Metadata())
	require.False(t, state.IsTerminal())
}

// 購読していない集約のイベントは状態を変えない
func TestAvatar_IgnoresOtherAggregateEvents(t *testing.T) {
	state := readmodel.Avatar{AvatarID: uuid.New(), Name: "アリス"}

	evolved := state.Evolve(&player.RegisteredEvent{EventMeta: player.EventMeta{EntityID: playertest.ID(t, 1), AggregateVersion: 1}})

	require.Equal(t, state, evolved)
}
