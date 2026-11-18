package constraint_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset/npcmindsettest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issueNPCMindsetID(t *testing.T) npcmindset.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, npcmindset.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func (f fixture) updateNPCMindsetName(t *testing.T, id npcmindset.ID, name string) error {
	t.Helper()
	return constraint.UpdateNPCMindsetName(f.ctx, id, npcmindsettest.Name(t, name))
}

// NPC の思考パターンの名前は 20 文字までで、テストごとの UUID を付けられないため、自分だけの DB で動かす（共有の core_test の名前とぶつからない）。
// 比べ方はカード名と同じ（domain.NormalizeName）なので、正規化の細かい場合はカード名のテストに任せ、ここでは代表だけを確かめる
func TestUpdateNPCMindsetName_RejectsNameUsedByOtherNPCMindset(t *testing.T) {
	f := isolatedSetup(t)
	require.NoError(t, f.updateNPCMindsetName(t, f.issueNPCMindsetID(t), "ABC"))

	err := f.updateNPCMindsetName(t, f.issueNPCMindsetID(t), " "+"ａｂｃ")

	require.ErrorIs(t, err, npcmindset.ErrNameAlreadyInUse)
}

func TestUpdateNPCMindsetName_AllowsNameUsedByNPCAvatarAndBoardAndCard(t *testing.T) {
	f := isolatedSetup(t)
	require.NoError(t, f.updateBoardName(t, f.issueBoardID(t), "草原"))
	require.NoError(t, f.updateCardName(t, f.issueCreatureID(t), "ドラゴン"))
	require.NoError(t, f.updateNPCAvatarName(t, f.issueNPCAvatarID(t), "ゴリガン"))

	require.NoError(t, f.updateNPCMindsetName(t, f.issueNPCMindsetID(t), "草原"))
	require.NoError(t, f.updateNPCMindsetName(t, f.issueNPCMindsetID(t), "ドラゴン"))
	require.NoError(t, f.updateNPCMindsetName(t, f.issueNPCMindsetID(t), "ゴリガン"))
}

func TestUpdateNPCMindsetName_ChangesOwnName(t *testing.T) {
	f := isolatedSetup(t)
	id := f.issueNPCMindsetID(t)
	require.NoError(t, f.updateNPCMindsetName(t, id, "森の番人"))

	require.NoError(t, f.updateNPCMindsetName(t, id, "炎の剣士"))

	require.NoError(t, f.updateNPCMindsetName(t, f.issueNPCMindsetID(t), "森の番人"))
}

// 削除（識別子の削除）で名前が空く
func TestUpdateNPCMindsetName_ReleasedWhenNPCMindsetIsDeleted(t *testing.T) {
	f := isolatedSetup(t)
	id := f.issueNPCMindsetID(t)
	require.NoError(t, f.updateNPCMindsetName(t, id, "森の番人"))

	require.NoError(t, identifier.New(f.registry, npcmindset.NewID).Delete(f.ctx, id))

	require.NoError(t, f.updateNPCMindsetName(t, f.issueNPCMindsetID(t), "森の番人"))
}
