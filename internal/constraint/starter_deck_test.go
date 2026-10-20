package constraint_test

import (
	"testing"

	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck/starterdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issueStarterDeckID(t *testing.T) starterdeck.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, starterdeck.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func (f fixture) updateStarterDeckName(t *testing.T, id starterdeck.ID, name string) error {
	t.Helper()
	return constraint.UpdateStarterDeckName(f.ctx, id, starterdecktest.Name(t, name))
}

// 比べ方はカード名と同じ（card.NormalizeName）なので、正規化の細かい場合はカード名のテストに任せ、ここでは代表だけを確かめる
func TestUpdateStarterDeckName_RejectsNameUsedByOtherStarterDeck(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updateStarterDeckName(t, f.issueStarterDeckID(t), cardName(t, "ABC")))

	err := f.updateStarterDeckName(t, f.issueStarterDeckID(t), " "+cardName(t, "ａｂｃ"))

	require.ErrorIs(t, err, starterdeck.ErrNameAlreadyInUse)
}

func TestUpdateStarterDeckName_AllowsNameUsedByCard(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updateCardName(t, f.issueCreatureID(t), cardName(t, "ドラゴン")))

	err := f.updateStarterDeckName(t, f.issueStarterDeckID(t), cardName(t, "ドラゴン"))

	require.NoError(t, err)
}

func TestUpdateStarterDeckName_ChangesOwnName(t *testing.T) {
	f := setup(t)
	id := f.issueStarterDeckID(t)
	require.NoError(t, f.updateStarterDeckName(t, id, cardName(t, "初心者")))

	require.NoError(t, f.updateStarterDeckName(t, id, cardName(t, "中級者")))

	// 前の名前は手放しているので、ほかのスターターデッキが使える
	require.NoError(t, f.updateStarterDeckName(t, f.issueStarterDeckID(t), cardName(t, "初心者")))
}

func (f fixture) cardRef(t *testing.T, id creature.ID) starterdeck.CardRef {
	t.Helper()
	return starterdecktest.CardRef(t, creature.Types().EntityType, id.Int64())
}

func (f fixture) referencedCards(t *testing.T, id starterdeck.ID) []int64 {
	t.Helper()
	return constrainttest.Referenced(t, f.ctx, starterdeck.CardReferenceType(), id.Int64())
}

// 変えた後の一覧にそろえる（無いものは足し、外れたものは消し、同じものはそのまま）
func TestUpdateStarterDeckCardReferences_MatchesGivenCards(t *testing.T) {
	f := setup(t)
	deck := f.issueStarterDeckID(t)
	dragon, goblin, slime := f.issueCreatureID(t), f.issueCreatureID(t), f.issueCreatureID(t)
	require.NoError(t, constraint.UpdateStarterDeckCardReferences(f.ctx, deck, []starterdeck.CardRef{f.cardRef(t, dragon), f.cardRef(t, goblin)}))

	err := constraint.UpdateStarterDeckCardReferences(f.ctx, deck, []starterdeck.CardRef{f.cardRef(t, goblin), f.cardRef(t, slime)})

	require.NoError(t, err)
	require.Equal(t, []int64{goblin.Int64(), slime.Int64()}, f.referencedCards(t, deck))
}

func TestUpdateStarterDeckCardReferences_RemovesAllWhenNoCards(t *testing.T) {
	f := setup(t)
	deck := f.issueStarterDeckID(t)
	require.NoError(t, constraint.UpdateStarterDeckCardReferences(f.ctx, deck, []starterdeck.CardRef{f.cardRef(t, f.issueCreatureID(t))}))

	err := constraint.UpdateStarterDeckCardReferences(f.ctx, deck, []starterdeck.CardRef{})

	require.NoError(t, err)
	require.Empty(t, f.referencedCards(t, deck))
}

// 参照されているカードの識別子は消せない（参照を確かめ忘れたコマンドがあっても、投影などが壊れないように）
func TestUpdateStarterDeckCardReferences_PreventsDeletingReferencedCard(t *testing.T) {
	f := setup(t)
	card := f.issueCreatureID(t)
	require.NoError(t, constraint.UpdateStarterDeckCardReferences(f.ctx, f.issueStarterDeckID(t), []starterdeck.CardRef{f.cardRef(t, card)}))

	err := identifier.New(f.registry, creature.NewID).Delete(f.ctx, card)

	// 失敗した文でトランザクションは中断されるが、テストの最後なので続けて使わない
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pqerror.RestrictViolation, pgErr.Code)
}

// 参照する側を消すと参照も消え、参照されていたカードを消せるようになる
func TestUpdateStarterDeckCardReferences_ReleasedWhenStarterDeckIsDeleted(t *testing.T) {
	f := setup(t)
	deck, card := f.issueStarterDeckID(t), f.issueCreatureID(t)
	require.NoError(t, constraint.UpdateStarterDeckCardReferences(f.ctx, deck, []starterdeck.CardRef{f.cardRef(t, card)}))

	require.NoError(t, identifier.New(f.registry, starterdeck.NewID).Delete(f.ctx, deck))

	require.Empty(t, f.referencedCards(t, deck))
	require.NoError(t, identifier.New(f.registry, creature.NewID).Delete(f.ctx, card))
}
