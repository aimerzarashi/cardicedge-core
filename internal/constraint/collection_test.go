package constraint_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issueCollection(t *testing.T) collection.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, collection.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func TestUpdateCollectionOwner_RejectsOwnerWhoAlreadyHasCollection(t *testing.T) {
	f := setup(t)
	playerID := f.issuePlayer(t)
	require.NoError(t, constraint.UpdateCollectionPlayer(f.ctx, f.issueCollection(t), playerID))

	err := constraint.UpdateCollectionPlayer(f.ctx, f.issueCollection(t), f.issuePlayer(t))
	require.NoError(t, err)
	err = constraint.UpdateCollectionPlayer(f.ctx, f.issueCollection(t), playerID)

	require.ErrorIs(t, err, collection.ErrAlreadyCreated)
}
