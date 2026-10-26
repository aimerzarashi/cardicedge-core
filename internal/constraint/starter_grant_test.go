package constraint_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issueStarterGrant(t *testing.T) startergrant.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, startergrant.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func TestUpdateStarterGrantPlayer_RejectsPlayerAlreadyGranted(t *testing.T) {
	f := setup(t)
	playerID := f.issuePlayer(t)
	first := f.issueStarterGrant(t)
	require.NoError(t, constraint.UpdateStarterGrantPlayer(f.ctx, first, playerID))

	err := constraint.UpdateStarterGrantPlayer(f.ctx, f.issueStarterGrant(t), playerID)

	require.ErrorIs(t, err, startergrant.ErrAlreadyStarted)
	require.Equal(t, first.Int64(), constrainttest.Owner(t, f.ctx, startergrant.PlayerKey(playerID)))
}
