package constraint_test

import (
	"testing"

	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck/presetdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issueGrantDeckID(t *testing.T) grantdeck.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, grantdeck.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func (f fixture) updateGrantDeckName(t *testing.T, id grantdeck.ID, name string) error {
	t.Helper()
	return constraint.UpdateGrantDeckName(f.ctx, id, presetdecktest.Name(t, name))
}

// 比べ方はカード名と同じ（domain.NormalizeName）なので、正規化の細かい場合はカード名のテストに任せ、ここでは代表だけを確かめる
func TestUpdateGrantDeckName_RejectsNameUsedByOtherGrantDeck(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updateGrantDeckName(t, f.issueGrantDeckID(t), cardName(t, "ABC")))

	err := f.updateGrantDeckName(t, f.issueGrantDeckID(t), " "+cardName(t, "ａｂｃ"))

	require.ErrorIs(t, err, grantdeck.ErrNameAlreadyInUse)
}

func TestUpdateGrantDeckName_AllowsNameUsedByPresetDeckAndCard(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updatePresetDeckName(t, f.issuePresetDeckID(t), cardName(t, "初心者")))
	require.NoError(t, f.updateCardName(t, f.issueCreatureID(t), cardName(t, "ドラゴン")))

	require.NoError(t, f.updateGrantDeckName(t, f.issueGrantDeckID(t), cardName(t, "初心者")))
	require.NoError(t, f.updateGrantDeckName(t, f.issueGrantDeckID(t), cardName(t, "ドラゴン")))
}

func TestUpdateGrantDeckName_ChangesOwnName(t *testing.T) {
	f := setup(t)
	id := f.issueGrantDeckID(t)
	require.NoError(t, f.updateGrantDeckName(t, id, cardName(t, "初心者")))

	require.NoError(t, f.updateGrantDeckName(t, id, cardName(t, "中級者")))

	require.NoError(t, f.updateGrantDeckName(t, f.issueGrantDeckID(t), cardName(t, "初心者")))
}

// 削除（識別子の削除）で名前が空く
func TestUpdateGrantDeckName_ReleasedWhenGrantDeckIsDeleted(t *testing.T) {
	f := setup(t)
	id := f.issueGrantDeckID(t)
	require.NoError(t, f.updateGrantDeckName(t, id, cardName(t, "初心者")))

	require.NoError(t, identifier.New(f.registry, grantdeck.NewID).Delete(f.ctx, id))

	require.NoError(t, f.updateGrantDeckName(t, f.issueGrantDeckID(t), cardName(t, "初心者")))
}

func (f fixture) grantDeckReferencedCards(t *testing.T, id grantdeck.ID) []int64 {
	t.Helper()
	return constrainttest.Referenced(t, f.ctx, grantdeck.CardReferenceType(), id.Int64())
}

func TestUpdateGrantDeckCardReferences_MatchesGivenCards(t *testing.T) {
	f := setup(t)
	deck := f.issueGrantDeckID(t)
	dragon, goblin, slime := f.issueCreatureID(t), f.issueCreatureID(t), f.issueCreatureID(t)
	require.NoError(t, constraint.UpdateGrantDeckCardReferences(f.ctx, deck, []presetdeck.CardRef{f.cardRef(t, dragon), f.cardRef(t, goblin)}))

	err := constraint.UpdateGrantDeckCardReferences(f.ctx, deck, []presetdeck.CardRef{f.cardRef(t, goblin), f.cardRef(t, slime)})

	require.NoError(t, err)
	require.Equal(t, []int64{goblin.Int64(), slime.Int64()}, f.grantDeckReferencedCards(t, deck))
}

// 参照されているカードの識別子は消せない。参照する側を消すと参照も消え、消せるようになる
func TestUpdateGrantDeckCardReferences_PreventsDeletingReferencedCardUntilGrantDeckIsDeleted(t *testing.T) {
	f := setup(t)
	deck, card := f.issueGrantDeckID(t), f.issueCreatureID(t)
	require.NoError(t, constraint.UpdateGrantDeckCardReferences(f.ctx, deck, []presetdeck.CardRef{f.cardRef(t, card)}))
	require.NoError(t, identifier.New(f.registry, grantdeck.NewID).Delete(f.ctx, deck))
	require.Empty(t, f.grantDeckReferencedCards(t, deck))
	require.NoError(t, constraint.UpdateGrantDeckCardReferences(f.ctx, f.issueGrantDeckID(t), []presetdeck.CardRef{f.cardRef(t, card)}))

	err := identifier.New(f.registry, creature.NewID).Delete(f.ctx, card)

	// 失敗した文でトランザクションは中断されるが、テストの最後なので続けて使わない
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pqerror.RestrictViolation, pgErr.Code)
}
