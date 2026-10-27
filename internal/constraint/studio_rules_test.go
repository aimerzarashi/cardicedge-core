package constraint_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issueStudioRulesID(t *testing.T) studiorules.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, studiorules.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

// 同時に動くほかのテストも、共有の core_test にstudio_rulesを作るため、自分の DB（テンプレートの複製）で確かめる
func TestCreateStudioRules_AllowsOnlyOne(t *testing.T) {
	f := isolatedSetup(t)
	first := f.issueStudioRulesID(t)
	require.NoError(t, constraint.CreateStudioRules(f.ctx, first))

	err := constraint.CreateStudioRules(f.ctx, f.issueStudioRulesID(t))

	require.ErrorIs(t, err, studiorules.ErrAlreadyCreated)
	found, ok, err := constraint.FindStudioRules(f.ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, first, found)
}

func TestFindStudioRules_ReturnsFalseBeforeCreated(t *testing.T) {
	f := isolatedSetup(t)

	_, ok, err := constraint.FindStudioRules(f.ctx)

	require.NoError(t, err)
	require.False(t, ok)
}
