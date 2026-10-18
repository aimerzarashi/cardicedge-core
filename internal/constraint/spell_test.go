package constraint_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issueSpellID(t *testing.T) spell.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, spell.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func (f fixture) updateSpellName(t *testing.T, id spell.ID, name string) error {
	t.Helper()
	return constraint.UpdateSpellName(f.ctx, id, cardtest.Name(t, name))
}

func TestUpdateSpellName_RejectsNameUsedByOtherCard(t *testing.T) {
	tests := []struct {
		name  string
		used  func(t *testing.T) string
		again func(t *testing.T) string
	}{
		{
			name:  "same_name",
			used:  func(t *testing.T) string { return cardName(t, "ドラゴン") },
			again: func(t *testing.T) string { return cardName(t, "ドラゴン") },
		},
		{
			name:  "full_width_and_half_width",
			used:  func(t *testing.T) string { return cardName(t, "ABC") },
			again: func(t *testing.T) string { return cardName(t, "ＡＢＣ") },
		},
		{
			name:  "upper_case_and_lower_case",
			used:  func(t *testing.T) string { return cardName(t, "Apple") },
			again: func(t *testing.T) string { return cardName(t, "apple") },
		},
		{
			name:  "surrounding_whitespace",
			used:  func(t *testing.T) string { return cardName(t, "ドラゴン") },
			again: func(t *testing.T) string { return "  " + cardName(t, "ドラゴン") + " " },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := setup(t)
			require.NoError(t, f.updateSpellName(t, f.issueSpellID(t), tt.used(t)))

			err := f.updateSpellName(t, f.issueSpellID(t), tt.again(t))

			require.ErrorIs(t, err, card.ErrNameAlreadyInUse)
		})
	}
}

func TestUpdateSpellName_AllowsDifferentName(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updateSpellName(t, f.issueSpellID(t), cardName(t, "ドラゴン")))

	err := f.updateSpellName(t, f.issueSpellID(t), cardName(t, "ゴブリン"))

	require.NoError(t, err)
}

func TestUpdateSpellName_ChangesOwnName(t *testing.T) {
	f := setup(t)
	id := f.issueSpellID(t)
	require.NoError(t, f.updateSpellName(t, id, cardName(t, "ドラゴン")))

	require.NoError(t, f.updateSpellName(t, id, cardName(t, "ゴブリン")))
	// 同じ値にし直しても、ほかのカードが使っているとは扱わない
	require.NoError(t, f.updateSpellName(t, id, cardName(t, "ゴブリン")))

	// 前の名前は手放しているので、ほかのカードが使える
	require.NoError(t, f.updateSpellName(t, f.issueSpellID(t), cardName(t, "ドラゴン")))
}
