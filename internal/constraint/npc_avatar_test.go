package constraint_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar/npcavatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issueNPCAvatarID(t *testing.T) npcavatar.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, npcavatar.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func (f fixture) updateNPCAvatarName(t *testing.T, id npcavatar.ID, name string) error {
	t.Helper()
	return constraint.UpdateNPCAvatarName(f.ctx, id, npcavatartest.Name(t, name))
}

// NPC のアバターの名前は 20 文字までで、テストごとの UUID を付けられないため、自分だけの DB で動かす（共有の core_test の名前とぶつからない）。
// 比べ方はカード名と同じ（domain.NormalizeName）なので、正規化の細かい場合はカード名のテストに任せ、ここでは代表だけを確かめる
func TestUpdateNPCAvatarName_RejectsNameUsedByOtherNPCAvatar(t *testing.T) {
	f := isolatedSetup(t)
	require.NoError(t, f.updateNPCAvatarName(t, f.issueNPCAvatarID(t), "ABC"))

	err := f.updateNPCAvatarName(t, f.issueNPCAvatarID(t), " "+"ａｂｃ")

	require.ErrorIs(t, err, npcavatar.ErrNameAlreadyInUse)
}

func TestUpdateNPCAvatarName_AllowsNameUsedByBoardAndCard(t *testing.T) {
	f := isolatedSetup(t)
	require.NoError(t, f.updateBoardName(t, f.issueBoardID(t), "草原"))
	require.NoError(t, f.updateCardName(t, f.issueCreatureID(t), "ドラゴン"))

	require.NoError(t, f.updateNPCAvatarName(t, f.issueNPCAvatarID(t), "草原"))
	require.NoError(t, f.updateNPCAvatarName(t, f.issueNPCAvatarID(t), "ドラゴン"))
}

func TestUpdateNPCAvatarName_ChangesOwnName(t *testing.T) {
	f := isolatedSetup(t)
	id := f.issueNPCAvatarID(t)
	require.NoError(t, f.updateNPCAvatarName(t, id, "森の番人"))

	require.NoError(t, f.updateNPCAvatarName(t, id, "炎の剣士"))

	require.NoError(t, f.updateNPCAvatarName(t, f.issueNPCAvatarID(t), "森の番人"))
}

// 削除（識別子の削除）で名前が空く
func TestUpdateNPCAvatarName_ReleasedWhenNPCAvatarIsDeleted(t *testing.T) {
	f := isolatedSetup(t)
	id := f.issueNPCAvatarID(t)
	require.NoError(t, f.updateNPCAvatarName(t, id, "森の番人"))

	require.NoError(t, identifier.New(f.registry, npcavatar.NewID).Delete(f.ctx, id))

	require.NoError(t, f.updateNPCAvatarName(t, f.issueNPCAvatarID(t), "森の番人"))
}
