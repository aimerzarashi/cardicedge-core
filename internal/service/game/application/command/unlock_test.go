package command_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/unlock"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

func readUnlockCreated(t *testing.T, ctx context.Context, playerID player.ID) (unlock.ID, unlock.CreatedEvent) {
	t.Helper()
	id, err := unlock.NewID(constrainttest.Owner(t, ctx, unlock.PlayerKey(playerID)))
	require.NoError(t, err)
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var raw []byte
	require.NoError(t, exec.QueryRowContext(ctx, `SELECT payload FROM domain_events WHERE entity_id = $1 AND aggregate_version = 1`, id.Int64()).Scan(&raw))
	var created unlock.CreatedEvent
	require.NoError(t, json.Unmarshal(raw, &created))
	return id, created
}

func TestCreateUnlock_CreatesUnlockOfOwnerWithoutBoards(t *testing.T) {
	ctx := isolatedDatabase(t)
	playerID := playertest.ID(t, issuePlayerID(t, ctx))

	require.NoError(t, newUnlockHandler(t, ctx).Create(ctx, playerID))

	_, created := readUnlockCreated(t, ctx, playerID)
	require.Equal(t, playerID, created.Player)
	require.Equal(t, 1, countEventsOf(t, ctx, unlockTypeID))
}

// プレイヤーに解放は 1 つ。2 回目はエラーにせず何もしない（ポリシーのやり直しで同じ作成が来ても害が無いように）
func TestCreateUnlock_DoesNothingWhenOwnerAlreadyHasUnlock(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newUnlockHandler(t, ctx)
	playerID := playertest.ID(t, issuePlayerID(t, ctx))
	require.NoError(t, handler.Create(ctx, playerID))

	err := handler.Create(ctx, playerID)

	require.NoError(t, err)
	require.Equal(t, 1, countEventsOf(t, ctx, unlockTypeID))
}

func TestUnlockBoards_UnlocksOnlyNewBoardsAndReportsWhetherAnythingChanged(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newUnlockHandler(t, ctx)
	playerID := playertest.ID(t, issuePlayerID(t, ctx))
	require.NoError(t, handler.Create(ctx, playerID))
	id, _ := readUnlockCreated(t, ctx, playerID)
	a, b := boardtest.ID(t, boardA), boardtest.ID(t, boardB)

	first, err := handler.UnlockBoards(ctx, id, []board.ID{b, a})
	require.NoError(t, err)
	again, err := handler.UnlockBoards(ctx, id, []board.ID{a})
	require.NoError(t, err)

	require.True(t, first)
	require.False(t, again)
	require.Equal(t, 2, countEventsOf(t, ctx, unlockTypeID))
	owner, err := handler.Player(ctx, id)
	require.NoError(t, err)
	require.Equal(t, playerID, owner)
}
