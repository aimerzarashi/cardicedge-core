package constraint_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issueGameRulesID(t *testing.T) gamerules.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, gamerules.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

// 同時に動くほかのテストも、共有の core_test にゲーム全体のルールを作るため、自分の DB（テンプレートの複製）で確かめる
func TestCreateGameRules_AllowsOnlyOne(t *testing.T) {
	f := isolatedSetup(t)
	first := f.issueGameRulesID(t)
	require.NoError(t, constraint.CreateGameRules(f.ctx, first))

	err := constraint.CreateGameRules(f.ctx, f.issueGameRulesID(t))

	require.ErrorIs(t, err, gamerules.ErrAlreadyCreated)
	found, ok, err := constraint.FindGameRules(f.ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, first, found)
}

func TestFindGameRules_ReturnsFalseBeforeCreated(t *testing.T) {
	f := isolatedSetup(t)

	_, ok, err := constraint.FindGameRules(f.ctx)

	require.NoError(t, err)
	require.False(t, ok)
}
