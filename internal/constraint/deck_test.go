package constraint_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issueDeck(t *testing.T) deck.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, deck.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func TestUpdateDeckStarter_RejectsSameNumberOfSameGrantOnly(t *testing.T) {
	f := setup(t)
	grant := f.issueStarterGrant(t)
	require.NoError(t, constraint.UpdateDeckStarter(f.ctx, f.issueDeck(t), grant, startergranttest.DeckNumber(t, 1)))

	require.NoError(t, constraint.UpdateDeckStarter(f.ctx, f.issueDeck(t), grant, startergranttest.DeckNumber(t, 2)))
	require.NoError(t, constraint.UpdateDeckStarter(f.ctx, f.issueDeck(t), f.issueStarterGrant(t), startergranttest.DeckNumber(t, 1)))
	err := constraint.UpdateDeckStarter(f.ctx, f.issueDeck(t), grant, startergranttest.DeckNumber(t, 1))

	require.ErrorIs(t, err, deck.ErrAlreadyCreated)
}
