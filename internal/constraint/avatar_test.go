package constraint_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar/avatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issueAvatar(t *testing.T) avatar.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, avatar.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func TestUpdateAvatarName_RejectsSameNormalizedNameOfSameOwnerOnly(t *testing.T) {
	f := setup(t)
	owner := f.issuePlayer(t)
	require.NoError(t, constraint.UpdateAvatarName(f.ctx, f.issueAvatar(t), owner, avatartest.Name(t, "ABC")))

	err := constraint.UpdateAvatarName(f.ctx, f.issueAvatar(t), owner, avatartest.Name(t, " ａｂｃ"))

	require.ErrorIs(t, err, avatar.ErrNameAlreadyInUse)
	require.NoError(t, constraint.UpdateAvatarName(f.ctx, f.issueAvatar(t), f.issuePlayer(t), avatartest.Name(t, "ABC")))
}

func TestReserveAvatarQuota_ReservesUpToLimitThenReportsLimitReached(t *testing.T) {
	f := setup(t)
	owner := f.issuePlayer(t)
	limit := quotaLimit(t, 3)
	for range 3 {
		require.NoError(t, constraint.ReserveAvatarQuota(f.ctx, avatar.PlayerQuota(owner), limit))
	}

	err := constraint.ReserveAvatarQuota(f.ctx, avatar.PlayerQuota(owner), limit)

	require.ErrorIs(t, err, avatar.ErrLimitReached)
}

// アバターの数は、デッキの数とは別の種別で数える
func TestReserveAvatarQuota_CountsSeparatelyFromDecks(t *testing.T) {
	f := setup(t)
	owner := f.issuePlayer(t)
	limit := quotaLimit(t, 1)
	require.NoError(t, constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(owner), limit))

	require.NoError(t, constraint.ReserveAvatarQuota(f.ctx, avatar.PlayerQuota(owner), limit))
}

func TestReserveAvatarQuota_CountsPerOwner(t *testing.T) {
	f := setup(t)
	first, second := f.issuePlayer(t), f.issuePlayer(t)
	limit := quotaLimit(t, 1)
	require.NoError(t, constraint.ReserveAvatarQuota(f.ctx, avatar.PlayerQuota(first), limit))

	require.NoError(t, constraint.ReserveAvatarQuota(f.ctx, avatar.PlayerQuota(second), limit))
}

func TestReserveAvatarQuota_DoesNotExceedLimitWhenReservedConcurrently(t *testing.T) {
	f := isolatedSetup(t)
	owner := f.issuePlayer(t)
	limit := quotaLimit(t, 5)
	var succeeded, reached atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := constraint.ReserveAvatarQuota(f.ctx, avatar.PlayerQuota(owner), limit)
			switch {
			case err == nil:
				succeeded.Add(1)
			case errors.Is(err, avatar.ErrLimitReached):
				reached.Add(1)
			}
		}()
	}
	wg.Wait()

	require.Equal(t, int32(5), succeeded.Load())
	require.Equal(t, int32(15), reached.Load())
}
