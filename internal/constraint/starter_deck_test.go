package constraint_test

import (
	"testing"

	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck/presetdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

func (f fixture) issueStarterDeckID(t *testing.T) starterdeck.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, starterdeck.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func (f fixture) updateStarterDeckName(t *testing.T, id starterdeck.ID, name string) error {
	t.Helper()
	return constraint.UpdateStarterDeckName(f.ctx, id, presetdecktest.Name(t, name))
}

// 比べ方はカード名と同じ（domain.NormalizeName）なので、正規化の細かい場合はカード名のテストに任せ、ここでは代表だけを確かめる
func TestUpdateStarterDeckName_RejectsNameUsedByOtherStarterDeck(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updateStarterDeckName(t, f.issueStarterDeckID(t), cardName(t, "ABC")))

	err := f.updateStarterDeckName(t, f.issueStarterDeckID(t), " "+cardName(t, "ａｂｃ"))

	require.ErrorIs(t, err, starterdeck.ErrNameAlreadyInUse)
}

func TestUpdateStarterDeckName_AllowsNameUsedByPresetDeckAndCard(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updatePresetDeckName(t, f.issuePresetDeckID(t), cardName(t, "初心者")))
	require.NoError(t, f.updateCardName(t, f.issueCreatureID(t), cardName(t, "ドラゴン")))

	require.NoError(t, f.updateStarterDeckName(t, f.issueStarterDeckID(t), cardName(t, "初心者")))
	require.NoError(t, f.updateStarterDeckName(t, f.issueStarterDeckID(t), cardName(t, "ドラゴン")))
}

func TestUpdateStarterDeckName_ChangesOwnName(t *testing.T) {
	f := setup(t)
	id := f.issueStarterDeckID(t)
	require.NoError(t, f.updateStarterDeckName(t, id, cardName(t, "初心者")))

	require.NoError(t, f.updateStarterDeckName(t, id, cardName(t, "中級者")))

	require.NoError(t, f.updateStarterDeckName(t, f.issueStarterDeckID(t), cardName(t, "初心者")))
}

// 削除（識別子の削除）で名前が空く
func TestUpdateStarterDeckName_ReleasedWhenStarterDeckIsDeleted(t *testing.T) {
	f := setup(t)
	id := f.issueStarterDeckID(t)
	require.NoError(t, f.updateStarterDeckName(t, id, cardName(t, "初心者")))

	require.NoError(t, identifier.New(f.registry, starterdeck.NewID).Delete(f.ctx, id))

	require.NoError(t, f.updateStarterDeckName(t, f.issueStarterDeckID(t), cardName(t, "初心者")))
}

func (f fixture) starterDeckReferencedCards(t *testing.T, id starterdeck.ID) []int64 {
	t.Helper()
	return constrainttest.Referenced(t, f.ctx, starterdeck.CardReferenceType(), id.Int64())
}

func TestUpdateStarterDeckCardReferences_MatchesGivenCards(t *testing.T) {
	f := setup(t)
	deck := f.issueStarterDeckID(t)
	dragon, goblin, slime := f.issueCreatureID(t), f.issueCreatureID(t), f.issueCreatureID(t)
	require.NoError(t, constraint.UpdateStarterDeckCardReferences(f.ctx, deck, []presetdeck.CardRef{f.cardRef(t, dragon), f.cardRef(t, goblin)}))

	err := constraint.UpdateStarterDeckCardReferences(f.ctx, deck, []presetdeck.CardRef{f.cardRef(t, goblin), f.cardRef(t, slime)})

	require.NoError(t, err)
	require.Equal(t, []int64{goblin.Int64(), slime.Int64()}, f.starterDeckReferencedCards(t, deck))
}

// 参照されているカードの識別子は消せない。参照する側を消すと参照も消え、消せるようになる
func TestUpdateStarterDeckCardReferences_PreventsDeletingReferencedCardUntilStarterDeckIsDeleted(t *testing.T) {
	f := setup(t)
	deck, card := f.issueStarterDeckID(t), f.issueCreatureID(t)
	require.NoError(t, constraint.UpdateStarterDeckCardReferences(f.ctx, deck, []presetdeck.CardRef{f.cardRef(t, card)}))
	require.NoError(t, identifier.New(f.registry, starterdeck.NewID).Delete(f.ctx, deck))
	require.Empty(t, f.starterDeckReferencedCards(t, deck))
	require.NoError(t, constraint.UpdateStarterDeckCardReferences(f.ctx, f.issueStarterDeckID(t), []presetdeck.CardRef{f.cardRef(t, card)}))

	err := identifier.New(f.registry, creature.NewID).Delete(f.ctx, card)

	// 失敗した文でトランザクションは中断されるが、テストの最後なので続けて使わない
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pqerror.RestrictViolation, pgErr.Code)
}

func (f fixture) starterDeckUsed(t *testing.T, ownerID int64) int {
	t.Helper()
	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	var used int
	err = exec.QueryRowContext(f.ctx, `
		SELECT COALESCE(SUM(q.used), 0) FROM domain_quotas q JOIN domain_quota_types t ON t.id = q.quota_type_id
		WHERE t.name = $1 AND q.owner_id = $2`, starterdeck.PublishedQuotaType.String(), ownerID).Scan(&used)
	require.NoError(t, err)
	return used
}

func TestReserveStarterDeckQuota_ReservesUpToLimitThenReportsLimitReached(t *testing.T) {
	f := setup(t)
	gameRulesID := f.issueGameRulesID(t)
	limit := quotaLimit(t, 2)
	for range 2 {
		require.NoError(t, constraint.ReserveStarterDeckQuota(f.ctx, starterdeck.PublishedQuota(gameRulesID), limit))
	}

	err := constraint.ReserveStarterDeckQuota(f.ctx, starterdeck.PublishedQuota(gameRulesID), limit)

	require.ErrorIs(t, err, starterdeck.ErrLimitReached)
	require.Equal(t, 2, f.starterDeckUsed(t, gameRulesID.Int64()))
}

func TestReleaseStarterDeckQuota_MakesRoomForNextPublish(t *testing.T) {
	f := setup(t)
	gameRulesID := f.issueGameRulesID(t)
	limit := quotaLimit(t, 1)
	require.NoError(t, constraint.ReserveStarterDeckQuota(f.ctx, starterdeck.PublishedQuota(gameRulesID), limit))

	require.NoError(t, constraint.ReleaseStarterDeckQuota(f.ctx, starterdeck.PublishedQuota(gameRulesID)))

	require.Equal(t, 0, f.starterDeckUsed(t, gameRulesID.Int64()))
	require.NoError(t, constraint.ReserveStarterDeckQuota(f.ctx, starterdeck.PublishedQuota(gameRulesID), limit))
}
