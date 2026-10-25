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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issuePresetDeckID(t *testing.T) presetdeck.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, presetdeck.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func (f fixture) updatePresetDeckName(t *testing.T, id presetdeck.ID, name string) error {
	t.Helper()
	return constraint.UpdatePresetDeckName(f.ctx, id, presetdecktest.Name(t, name))
}

// 比べ方はカード名と同じ（domain.NormalizeName）なので、正規化の細かい場合はカード名のテストに任せ、ここでは代表だけを確かめる
func TestUpdatePresetDeckName_RejectsNameUsedByOtherPresetDeck(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updatePresetDeckName(t, f.issuePresetDeckID(t), cardName(t, "ABC")))

	err := f.updatePresetDeckName(t, f.issuePresetDeckID(t), " "+cardName(t, "ａｂｃ"))

	require.ErrorIs(t, err, presetdeck.ErrNameAlreadyInUse)
}

func TestUpdatePresetDeckName_AllowsNameUsedByCard(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updateCardName(t, f.issueCreatureID(t), cardName(t, "ドラゴン")))

	err := f.updatePresetDeckName(t, f.issuePresetDeckID(t), cardName(t, "ドラゴン"))

	require.NoError(t, err)
}

func TestUpdatePresetDeckName_ChangesOwnName(t *testing.T) {
	f := setup(t)
	id := f.issuePresetDeckID(t)
	require.NoError(t, f.updatePresetDeckName(t, id, cardName(t, "初心者")))

	require.NoError(t, f.updatePresetDeckName(t, id, cardName(t, "中級者")))

	// 前の名前は手放しているので、ほかのプリセットデッキが使える
	require.NoError(t, f.updatePresetDeckName(t, f.issuePresetDeckID(t), cardName(t, "初心者")))
}

func (f fixture) referencedCards(t *testing.T, id presetdeck.ID) []int64 {
	t.Helper()
	return constrainttest.Referenced(t, f.ctx, presetdeck.CardReferenceType(), id.Int64())
}

// 変えた後の一覧にそろえる（無いものは足し、外れたものは消し、同じものはそのまま）
func TestUpdatePresetDeckCardReferences_MatchesGivenCards(t *testing.T) {
	f := setup(t)
	deck := f.issuePresetDeckID(t)
	dragon, goblin, slime := f.issueCreatureID(t), f.issueCreatureID(t), f.issueCreatureID(t)
	require.NoError(t, constraint.UpdatePresetDeckCardReferences(f.ctx, deck, []presetdeck.CardRef{f.cardRef(t, dragon), f.cardRef(t, goblin)}))

	err := constraint.UpdatePresetDeckCardReferences(f.ctx, deck, []presetdeck.CardRef{f.cardRef(t, goblin), f.cardRef(t, slime)})

	require.NoError(t, err)
	require.Equal(t, []int64{goblin.Int64(), slime.Int64()}, f.referencedCards(t, deck))
}

func TestUpdatePresetDeckCardReferences_RemovesAllWhenNoCards(t *testing.T) {
	f := setup(t)
	deck := f.issuePresetDeckID(t)
	require.NoError(t, constraint.UpdatePresetDeckCardReferences(f.ctx, deck, []presetdeck.CardRef{f.cardRef(t, f.issueCreatureID(t))}))

	err := constraint.UpdatePresetDeckCardReferences(f.ctx, deck, []presetdeck.CardRef{})

	require.NoError(t, err)
	require.Empty(t, f.referencedCards(t, deck))
}

// 参照されているカードの識別子は消せない（参照を確かめ忘れたコマンドがあっても、投影などが壊れないように）
func TestUpdatePresetDeckCardReferences_PreventsDeletingReferencedCard(t *testing.T) {
	f := setup(t)
	card := f.issueCreatureID(t)
	require.NoError(t, constraint.UpdatePresetDeckCardReferences(f.ctx, f.issuePresetDeckID(t), []presetdeck.CardRef{f.cardRef(t, card)}))

	err := identifier.New(f.registry, creature.NewID).Delete(f.ctx, card)

	// 失敗した文でトランザクションは中断されるが、テストの最後なので続けて使わない
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pqerror.RestrictViolation, pgErr.Code)
}

// 参照する側を消すと参照も消え、参照されていたカードを消せるようになる
func TestUpdatePresetDeckCardReferences_ReleasedWhenPresetDeckIsDeleted(t *testing.T) {
	f := setup(t)
	deck, card := f.issuePresetDeckID(t), f.issueCreatureID(t)
	require.NoError(t, constraint.UpdatePresetDeckCardReferences(f.ctx, deck, []presetdeck.CardRef{f.cardRef(t, card)}))

	require.NoError(t, identifier.New(f.registry, presetdeck.NewID).Delete(f.ctx, deck))

	require.Empty(t, f.referencedCards(t, deck))
	require.NoError(t, identifier.New(f.registry, creature.NewID).Delete(f.ctx, card))
}
