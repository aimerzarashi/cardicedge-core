package constraint_test

import (
	"testing"

	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterset/startersettest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

func (f fixture) issueStarterSetID(t *testing.T) starterset.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, starterset.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func (f fixture) updateStarterSetName(t *testing.T, id starterset.ID, name string) error {
	t.Helper()
	return constraint.UpdateStarterSetName(f.ctx, id, startersettest.Name(t, name))
}

// 比べ方はカード名と同じ（domain.NormalizeName）なので、正規化の細かい場合はカード名のテストに任せ、ここでは代表だけを確かめる。
// 名前は 20 文字までで、テストごとの UUID を付けられないため、自分だけの DB で動かす（共有の core_test の名前とぶつからない）
func TestUpdateStarterSetName_RejectsNameUsedByOtherStarterSet(t *testing.T) {
	f := isolatedSetup(t)
	require.NoError(t, f.updateStarterSetName(t, f.issueStarterSetID(t), "ABC"))

	err := f.updateStarterSetName(t, f.issueStarterSetID(t), " "+"ａｂｃ")

	require.ErrorIs(t, err, starterset.ErrNameAlreadyInUse)
}

// 削除（識別子の削除）で名前が空く
func TestUpdateStarterSetName_ReleasedWhenStarterSetIsDeleted(t *testing.T) {
	f := isolatedSetup(t)
	id := f.issueStarterSetID(t)
	require.NoError(t, f.updateStarterSetName(t, id, "標準"))

	require.NoError(t, identifier.New(f.registry, starterset.NewID).Delete(f.ctx, id))

	require.NoError(t, f.updateStarterSetName(t, f.issueStarterSetID(t), "標準"))
}

func TestUpdateStarterSetReferences_MatchGivenGrantDecksAndBoards(t *testing.T) {
	f := setup(t)
	set := f.issueStarterSetID(t)
	firstDeck, secondDeck, thirdDeck := f.issueGrantDeckID(t), f.issueGrantDeckID(t), f.issueGrantDeckID(t)
	firstBoard, secondBoard := f.issueBoardID(t), f.issueBoardID(t)
	require.NoError(t, constraint.UpdateStarterSetGrantDeckReferences(f.ctx, set, []grantdeck.ID{firstDeck, secondDeck}))
	require.NoError(t, constraint.UpdateStarterSetBoardReferences(f.ctx, set, []board.ID{firstBoard}))

	require.NoError(t, constraint.UpdateStarterSetGrantDeckReferences(f.ctx, set, []grantdeck.ID{secondDeck, thirdDeck}))
	require.NoError(t, constraint.UpdateStarterSetBoardReferences(f.ctx, set, []board.ID{secondBoard}))

	require.Equal(t, []int64{secondDeck.Int64(), thirdDeck.Int64()}, constrainttest.Referenced(t, f.ctx, starterset.GrantDeckReferenceType(), set.Int64()))
	require.Equal(t, []int64{secondBoard.Int64()}, constrainttest.Referenced(t, f.ctx, starterset.BoardReferenceType(), set.Int64()))
}

// 参照されているボードの識別子は消せない。参照する側を消すと参照も消え、消せるようになる
func TestUpdateStarterSetBoardReferences_PreventsDeletingReferencedBoardUntilStarterSetIsDeleted(t *testing.T) {
	f := setup(t)
	set, target := f.issueStarterSetID(t), f.issueBoardID(t)
	require.NoError(t, constraint.UpdateStarterSetBoardReferences(f.ctx, set, []board.ID{target}))
	require.NoError(t, identifier.New(f.registry, starterset.NewID).Delete(f.ctx, set))
	require.Empty(t, constrainttest.Referenced(t, f.ctx, starterset.BoardReferenceType(), set.Int64()))
	require.NoError(t, constraint.UpdateStarterSetBoardReferences(f.ctx, f.issueStarterSetID(t), []board.ID{target}))

	err := identifier.New(f.registry, board.NewID).Delete(f.ctx, target)

	// 失敗した文でトランザクションは中断されるが、テストの最後なので続けて使わない
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pqerror.RestrictViolation, pgErr.Code)
}

func TestUpdateStarterSetGrantDeckReferences_PreventsDeletingReferencedGrantDeckUntilStarterSetIsDeleted(t *testing.T) {
	f := setup(t)
	set, target := f.issueStarterSetID(t), f.issueGrantDeckID(t)
	require.NoError(t, constraint.UpdateStarterSetGrantDeckReferences(f.ctx, set, []grantdeck.ID{target}))
	require.NoError(t, identifier.New(f.registry, starterset.NewID).Delete(f.ctx, set))
	require.Empty(t, constrainttest.Referenced(t, f.ctx, starterset.GrantDeckReferenceType(), set.Int64()))
	require.NoError(t, constraint.UpdateStarterSetGrantDeckReferences(f.ctx, f.issueStarterSetID(t), []grantdeck.ID{target}))

	err := identifier.New(f.registry, grantdeck.NewID).Delete(f.ctx, target)

	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pqerror.RestrictViolation, pgErr.Code)
}

func TestUpdateStarterSetNPCAvatarReferences_PreventsDeletingReferencedNPCAvatarUntilStarterSetIsDeleted(t *testing.T) {
	f := setup(t)
	set, target := f.issueStarterSetID(t), f.issueNPCAvatarID(t)
	require.NoError(t, constraint.UpdateStarterSetNPCAvatarReferences(f.ctx, set, []npcavatar.ID{target}))
	require.Equal(t, []int64{target.Int64()}, constrainttest.Referenced(t, f.ctx, starterset.NPCAvatarReferenceType(), set.Int64()))
	require.NoError(t, identifier.New(f.registry, starterset.NewID).Delete(f.ctx, set))
	require.Empty(t, constrainttest.Referenced(t, f.ctx, starterset.NPCAvatarReferenceType(), set.Int64()))
	require.NoError(t, constraint.UpdateStarterSetNPCAvatarReferences(f.ctx, f.issueStarterSetID(t), []npcavatar.ID{target}))

	err := identifier.New(f.registry, npcavatar.NewID).Delete(f.ctx, target)

	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pqerror.RestrictViolation, pgErr.Code)
}

func (f fixture) starterSetUsed(t *testing.T, ownerID int64) int {
	t.Helper()
	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	var used int
	err = exec.QueryRowContext(f.ctx, `
		SELECT COALESCE(SUM(q.used), 0) FROM domain_quotas q JOIN domain_quota_types t ON t.id = q.quota_type_id
		WHERE t.name = $1 AND q.owner_id = $2`, starterset.PublishedQuotaType.String(), ownerID).Scan(&used)
	require.NoError(t, err)
	return used
}

func TestReserveStarterSetQuota_ReservesUpToLimitThenReportsLimitReached(t *testing.T) {
	f := setup(t)
	gameRulesID := f.issueGameRulesID(t)
	limit := quotaLimit(t, starterset.MaxPublished)
	require.NoError(t, constraint.ReserveStarterSetQuota(f.ctx, starterset.PublishedQuota(gameRulesID), limit))

	err := constraint.ReserveStarterSetQuota(f.ctx, starterset.PublishedQuota(gameRulesID), limit)

	require.ErrorIs(t, err, starterset.ErrLimitReached)
	require.Equal(t, 1, f.starterSetUsed(t, gameRulesID.Int64()))
}

func TestReleaseStarterSetQuota_MakesRoomForNextPublish(t *testing.T) {
	f := setup(t)
	gameRulesID := f.issueGameRulesID(t)
	limit := quotaLimit(t, starterset.MaxPublished)
	require.NoError(t, constraint.ReserveStarterSetQuota(f.ctx, starterset.PublishedQuota(gameRulesID), limit))

	require.NoError(t, constraint.ReleaseStarterSetQuota(f.ctx, starterset.PublishedQuota(gameRulesID)))

	require.Equal(t, 0, f.starterSetUsed(t, gameRulesID.Int64()))
	require.NoError(t, constraint.ReserveStarterSetQuota(f.ctx, starterset.PublishedQuota(gameRulesID), limit))
}
