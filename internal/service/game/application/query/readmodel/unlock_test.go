package readmodel_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/unlock"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/unlock/unlocktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

func TestUnlock_CreatedMakesRowWithoutBoardsAndBoardsUnlockedAppendsInOrder(t *testing.T) {
	boardA, boardB, boardC := boardtest.ID(t, 50), boardtest.ID(t, 51), boardtest.ID(t, 52)
	occurredAt := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	laterAt := occurredAt.Add(time.Hour)
	created := &unlock.CreatedEvent{
		EventMeta: unlock.EventMeta{EntityID: unlocktest.ID(t, 3), AggregateVersion: 1, OccurredAt: occurredAt},
		Player:    playertest.ID(t, 7),
	}
	first := &unlock.BoardsUnlockedEvent{
		EventMeta: unlock.EventMeta{EntityID: unlocktest.ID(t, 3), AggregateVersion: 2, OccurredAt: occurredAt},
		Boards:    []board.ID{boardB, boardA},
	}
	later := &unlock.BoardsUnlockedEvent{
		EventMeta: unlock.EventMeta{EntityID: unlocktest.ID(t, 3), AggregateVersion: 3, OccurredAt: laterAt},
		Boards:    []board.ID{boardC},
	}

	empty := readmodel.Unlock{}.Evolve(created)
	unlocked := empty.Evolve(first).Evolve(later)

	require.Equal(t, readmodel.Unlock{Unlock: unlocktest.ID(t, 3), Player: playertest.ID(t, 7), Boards: []board.ID{}, RegisteredAt: occurredAt, UpdatedAt: occurredAt}, empty)
	require.Equal(t, []board.ID{boardB, boardA, boardC}, unlocked.Boards)
	require.Equal(t, occurredAt, unlocked.RegisteredAt)
	// 公開 ID と操作した人は、イベントが内部 ID で持つため空のまま（書くときに repository が入れる）
	require.Equal(t, readmodel.UnlockMetadata{RegisteredAt: occurredAt, UpdatedAt: laterAt}, unlocked.Metadata())
	require.False(t, unlocked.IsTerminal())
}

// 購読していない集約のイベントは状態を変えない
func TestUnlock_IgnoresOtherAggregateEvents(t *testing.T) {
	state := readmodel.Unlock{Boards: []board.ID{boardtest.ID(t, 50)}}

	evolved := state.Evolve(&player.RegisteredEvent{EventMeta: player.EventMeta{EntityID: playertest.ID(t, 1), AggregateVersion: 1}})

	require.Equal(t, state, evolved)
}
