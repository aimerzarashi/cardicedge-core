package constraint_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issueBoardID(t *testing.T) board.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, board.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func (f fixture) updateBoardName(t *testing.T, id board.ID, name string) error {
	t.Helper()
	return constraint.UpdateBoardName(f.ctx, id, boardtest.Name(t, name))
}

// 比べ方はカード名と同じ（domain.NormalizeName）なので、正規化の細かい場合はカード名のテストに任せ、ここでは代表だけを確かめる
func TestUpdateBoardName_RejectsNameUsedByOtherBoard(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updateBoardName(t, f.issueBoardID(t), cardName(t, "ABC")))

	err := f.updateBoardName(t, f.issueBoardID(t), " "+cardName(t, "ａｂｃ"))

	require.ErrorIs(t, err, board.ErrNameAlreadyInUse)
}

func TestUpdateBoardName_AllowsNameUsedByStarterDeckAndCard(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updateStarterDeckName(t, f.issueStarterDeckID(t), cardName(t, "初心者")))
	require.NoError(t, f.updateCardName(t, f.issueCreatureID(t), cardName(t, "ドラゴン")))

	require.NoError(t, f.updateBoardName(t, f.issueBoardID(t), cardName(t, "初心者")))
	require.NoError(t, f.updateBoardName(t, f.issueBoardID(t), cardName(t, "ドラゴン")))
}

func TestUpdateBoardName_ChangesOwnName(t *testing.T) {
	f := setup(t)
	id := f.issueBoardID(t)
	require.NoError(t, f.updateBoardName(t, id, cardName(t, "草原")))

	require.NoError(t, f.updateBoardName(t, id, cardName(t, "砂漠")))

	require.NoError(t, f.updateBoardName(t, f.issueBoardID(t), cardName(t, "草原")))
}

// 削除（識別子の削除）で名前が空く
func TestUpdateBoardName_ReleasedWhenBoardIsDeleted(t *testing.T) {
	f := setup(t)
	id := f.issueBoardID(t)
	require.NoError(t, f.updateBoardName(t, id, cardName(t, "草原")))

	require.NoError(t, identifier.New(f.registry, board.NewID).Delete(f.ctx, id))

	require.NoError(t, f.updateBoardName(t, f.issueBoardID(t), cardName(t, "草原")))
}
