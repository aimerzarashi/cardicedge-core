package constraint_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

var cardNameSuffixes sync.Map

// 開発用の DB に残っているカードの名前とぶつからないよう、テストごとに UUID を付ける（同じテストの中では同じ値）
func cardName(t *testing.T, base string) string {
	t.Helper()
	suffix, _ := cardNameSuffixes.LoadOrStore(t.Name(), uuid.NewString())
	return fmt.Sprintf("%s-%s", base, suffix)
}

func (f fixture) issueCreatureID(t *testing.T) creature.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, creature.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func (f fixture) updateCardName(t *testing.T, id creature.ID, name string) error {
	t.Helper()
	return constraint.UpdateCreatureName(f.ctx, id, cardtest.Name(t, name))
}

func TestUpdateCreatureName_RejectsNameUsedByOtherCard(t *testing.T) {
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
			name:  "surrounding_whitespace",
			used:  func(t *testing.T) string { return cardName(t, "ドラゴン") },
			again: func(t *testing.T) string { return "  " + cardName(t, "ドラゴン") + " " },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := setup(t)
			require.NoError(t, f.updateCardName(t, f.issueCreatureID(t), tt.used(t)))

			err := f.updateCardName(t, f.issueCreatureID(t), tt.again(t))

			require.ErrorIs(t, err, card.ErrNameAlreadyInUse)
		})
	}
}

func TestUpdateCreatureName_AllowsDifferentName(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updateCardName(t, f.issueCreatureID(t), cardName(t, "ドラゴン")))

	err := f.updateCardName(t, f.issueCreatureID(t), cardName(t, "ゴブリン"))

	require.NoError(t, err)
}

func TestUpdateCreatureName_ChangesOwnName(t *testing.T) {
	f := setup(t)
	id := f.issueCreatureID(t)
	require.NoError(t, f.updateCardName(t, id, cardName(t, "ドラゴン")))

	require.NoError(t, f.updateCardName(t, id, cardName(t, "ゴブリン")))
	// 同じ値にし直しても、ほかのカードが使っているとは扱わない
	require.NoError(t, f.updateCardName(t, id, cardName(t, "ゴブリン")))

	// 前の名前は手放しているので、ほかのカードが使える
	require.NoError(t, f.updateCardName(t, f.issueCreatureID(t), cardName(t, "ドラゴン")))
}
