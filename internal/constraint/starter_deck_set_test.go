package constraint_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck/presetdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issueStarterDeckSetID(t *testing.T) starterdeckset.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, starterdeckset.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

// 同時に動くほかのテストも、共有の core_test にスターターデッキセットを作るため、自分の DB（テンプレートの複製）で確かめる
func TestCreateStarterDeckSet_AllowsOnlyOne(t *testing.T) {
	f := isolatedSetup(t)
	first := f.issueStarterDeckSetID(t)
	require.NoError(t, constraint.CreateStarterDeckSet(f.ctx, first))

	err := constraint.CreateStarterDeckSet(f.ctx, f.issueStarterDeckSetID(t))

	require.ErrorIs(t, err, starterdeckset.ErrAlreadyCreated)
	found, ok, err := constraint.FindStarterDeckSet(f.ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, first, found)
}

func TestFindStarterDeckSet_ReturnsFalseBeforeCreated(t *testing.T) {
	f := isolatedSetup(t)

	_, ok, err := constraint.FindStarterDeckSet(f.ctx)

	require.NoError(t, err)
	require.False(t, ok)
}

func TestUpdateStarterDeckSetCardReferences_MatchesGivenCards(t *testing.T) {
	f := setup(t)
	set := f.issueStarterDeckSetID(t)
	dragon, goblin := f.issueCreatureID(t), f.issueCreatureID(t)
	cardRef := func(id creature.ID) presetdeck.CardRef {
		return presetdecktest.CardRef(t, creature.Types().EntityType, id.Int64())
	}

	err := constraint.UpdateStarterDeckSetCardReferences(f.ctx, set, []presetdeck.CardRef{cardRef(dragon), cardRef(goblin)})

	require.NoError(t, err)
	require.Equal(t, []int64{dragon.Int64(), goblin.Int64()}, constrainttest.Referenced(t, f.ctx, starterdeckset.CardReferenceType(), set.Int64()))
}
