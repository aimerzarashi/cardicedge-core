package constraint_test

import (
	"testing"

	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck/presetdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issueNPCDeckID(t *testing.T) npcdeck.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, npcdeck.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func (f fixture) updateNPCDeckName(t *testing.T, id npcdeck.ID, name string) error {
	t.Helper()
	return constraint.UpdateNPCDeckName(f.ctx, id, presetdecktest.Name(t, name))
}

// 比べ方はカード名と同じ（domain.NormalizeName）なので、正規化の細かい場合はカード名のテストに任せ、ここでは代表だけを確かめる
func TestUpdateNPCDeckName_RejectsNameUsedByOtherNPCDeck(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updateNPCDeckName(t, f.issueNPCDeckID(t), cardName(t, "ABC")))

	err := f.updateNPCDeckName(t, f.issueNPCDeckID(t), " "+cardName(t, "ａｂｃ"))

	require.ErrorIs(t, err, npcdeck.ErrNameAlreadyInUse)
}

func TestUpdateNPCDeckName_AllowsNameUsedByPresetDeckGrantDeckAndCard(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updatePresetDeckName(t, f.issuePresetDeckID(t), cardName(t, "初心者")))
	require.NoError(t, f.updateGrantDeckName(t, f.issueGrantDeckID(t), cardName(t, "中級者")))
	require.NoError(t, f.updateCardName(t, f.issueCreatureID(t), cardName(t, "ドラゴン")))

	require.NoError(t, f.updateNPCDeckName(t, f.issueNPCDeckID(t), cardName(t, "初心者")))
	require.NoError(t, f.updateNPCDeckName(t, f.issueNPCDeckID(t), cardName(t, "中級者")))
	require.NoError(t, f.updateNPCDeckName(t, f.issueNPCDeckID(t), cardName(t, "ドラゴン")))
}

func TestUpdateNPCDeckName_ChangesOwnName(t *testing.T) {
	f := setup(t)
	id := f.issueNPCDeckID(t)
	require.NoError(t, f.updateNPCDeckName(t, id, cardName(t, "初心者")))

	require.NoError(t, f.updateNPCDeckName(t, id, cardName(t, "中級者")))

	require.NoError(t, f.updateNPCDeckName(t, f.issueNPCDeckID(t), cardName(t, "初心者")))
}

// 削除（識別子の削除）で名前が空く
func TestUpdateNPCDeckName_ReleasedWhenNPCDeckIsDeleted(t *testing.T) {
	f := setup(t)
	id := f.issueNPCDeckID(t)
	require.NoError(t, f.updateNPCDeckName(t, id, cardName(t, "初心者")))

	require.NoError(t, identifier.New(f.registry, npcdeck.NewID).Delete(f.ctx, id))

	require.NoError(t, f.updateNPCDeckName(t, f.issueNPCDeckID(t), cardName(t, "初心者")))
}

func (f fixture) npcDeckReferencedCards(t *testing.T, id npcdeck.ID) []int64 {
	t.Helper()
	return constrainttest.Referenced(t, f.ctx, npcdeck.CardReferenceType(), id.Int64())
}

func TestUpdateNPCDeckCardReferences_MatchesGivenCards(t *testing.T) {
	f := setup(t)
	npcDeck := f.issueNPCDeckID(t)
	dragon, goblin, slime := f.issueCreatureID(t), f.issueCreatureID(t), f.issueCreatureID(t)
	require.NoError(t, constraint.UpdateNPCDeckCardReferences(f.ctx, npcDeck, []card.Ref{f.cardRef(t, dragon), f.cardRef(t, goblin)}))

	err := constraint.UpdateNPCDeckCardReferences(f.ctx, npcDeck, []card.Ref{f.cardRef(t, goblin), f.cardRef(t, slime)})

	require.NoError(t, err)
	require.Equal(t, []int64{goblin.Int64(), slime.Int64()}, f.npcDeckReferencedCards(t, npcDeck))
}

// 参照されているカードの識別子は消せない。参照する側を消すと参照も消え、消せるようになる
func TestUpdateNPCDeckCardReferences_PreventsDeletingReferencedCardUntilNPCDeckIsDeleted(t *testing.T) {
	f := setup(t)
	npcDeck, creatureID := f.issueNPCDeckID(t), f.issueCreatureID(t)
	require.NoError(t, constraint.UpdateNPCDeckCardReferences(f.ctx, npcDeck, []card.Ref{f.cardRef(t, creatureID)}))
	require.NoError(t, identifier.New(f.registry, npcdeck.NewID).Delete(f.ctx, npcDeck))
	require.Empty(t, f.npcDeckReferencedCards(t, npcDeck))
	require.NoError(t, constraint.UpdateNPCDeckCardReferences(f.ctx, f.issueNPCDeckID(t), []card.Ref{f.cardRef(t, creatureID)}))

	err := identifier.New(f.registry, creature.NewID).Delete(f.ctx, creatureID)

	// 失敗した文でトランザクションは中断されるが、テストの最後なので続けて使わない
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pqerror.RestrictViolation, pgErr.Code)
}
