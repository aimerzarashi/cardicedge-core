package constraint_test

import (
	"testing"

	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar/npcavatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
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

func TestUpdateNPCAvatarDeckReferences_MatchesGivenDecks(t *testing.T) {
	f := setup(t)
	npcAvatar := f.issueNPCAvatarID(t)
	first, second, third := f.issueNPCDeckID(t), f.issueNPCDeckID(t), f.issueNPCDeckID(t)
	require.NoError(t, constraint.UpdateNPCAvatarDeckReferences(f.ctx, npcAvatar, []npcdeck.ID{first, second}))

	err := constraint.UpdateNPCAvatarDeckReferences(f.ctx, npcAvatar, []npcdeck.ID{second, third})

	require.NoError(t, err)
	require.Equal(t, []int64{second.Int64(), third.Int64()}, constrainttest.Referenced(t, f.ctx, npcavatar.DeckReferenceType(), npcAvatar.Int64()))
}

// 参照されている NPC のデッキの識別子は消せない。参照する側を消すと参照も消え、消せるようになる
func TestUpdateNPCAvatarDeckReferences_PreventsDeletingReferencedDeckUntilNPCAvatarIsDeleted(t *testing.T) {
	f := setup(t)
	npcAvatar, deck := f.issueNPCAvatarID(t), f.issueNPCDeckID(t)
	require.NoError(t, constraint.UpdateNPCAvatarDeckReferences(f.ctx, npcAvatar, []npcdeck.ID{deck}))
	require.NoError(t, identifier.New(f.registry, npcavatar.NewID).Delete(f.ctx, npcAvatar))
	require.Empty(t, constrainttest.Referenced(t, f.ctx, npcavatar.DeckReferenceType(), npcAvatar.Int64()))
	require.NoError(t, constraint.UpdateNPCAvatarDeckReferences(f.ctx, f.issueNPCAvatarID(t), []npcdeck.ID{deck}))

	err := identifier.New(f.registry, npcdeck.NewID).Delete(f.ctx, deck)

	// 失敗した文でトランザクションは中断されるが、テストの最後なので続けて使わない
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pqerror.RestrictViolation, pgErr.Code)
}

func TestUpdateNPCAvatarMindsetReferences_MatchesGivenMindsets(t *testing.T) {
	f := setup(t)
	npcAvatar := f.issueNPCAvatarID(t)
	first, second, third := f.issueNPCMindsetID(t), f.issueNPCMindsetID(t), f.issueNPCMindsetID(t)
	require.NoError(t, constraint.UpdateNPCAvatarMindsetReferences(f.ctx, npcAvatar, []npcmindset.ID{first, second}))

	err := constraint.UpdateNPCAvatarMindsetReferences(f.ctx, npcAvatar, []npcmindset.ID{second, third})

	require.NoError(t, err)
	require.Equal(t, []int64{second.Int64(), third.Int64()}, constrainttest.Referenced(t, f.ctx, npcavatar.MindsetReferenceType(), npcAvatar.Int64()))
}

// 参照されている NPC の思考パターンの識別子は消せない。参照する側を消すと参照も消え、消せるようになる
func TestUpdateNPCAvatarMindsetReferences_PreventsDeletingReferencedMindsetUntilNPCAvatarIsDeleted(t *testing.T) {
	f := setup(t)
	npcAvatar, mindset := f.issueNPCAvatarID(t), f.issueNPCMindsetID(t)
	require.NoError(t, constraint.UpdateNPCAvatarMindsetReferences(f.ctx, npcAvatar, []npcmindset.ID{mindset}))
	require.NoError(t, identifier.New(f.registry, npcavatar.NewID).Delete(f.ctx, npcAvatar))
	require.Empty(t, constrainttest.Referenced(t, f.ctx, npcavatar.MindsetReferenceType(), npcAvatar.Int64()))
	require.NoError(t, constraint.UpdateNPCAvatarMindsetReferences(f.ctx, f.issueNPCAvatarID(t), []npcmindset.ID{mindset}))

	err := identifier.New(f.registry, npcmindset.NewID).Delete(f.ctx, mindset)

	// 失敗した文でトランザクションは中断されるが、テストの最後なので続けて使わない
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pqerror.RestrictViolation, pgErr.Code)
}
