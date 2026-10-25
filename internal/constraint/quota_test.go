package constraint_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck/decktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

func quotaLimit(t *testing.T, value int) domain.QuotaLimit {
	t.Helper()
	limit, err := domain.NewQuotaLimit(value)
	require.NoError(t, err)
	return limit
}

func (f fixture) used(t *testing.T, owner player.ID) int {
	t.Helper()
	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	var used int
	err = exec.QueryRowContext(f.ctx, `
		SELECT COALESCE(SUM(q.used), 0) FROM domain_quotas q JOIN domain_quota_types t ON t.id = q.quota_type_id
		WHERE t.name = $1 AND q.owner_id = $2`, deck.PlayerQuotaType.String(), owner.Int64()).Scan(&used)
	require.NoError(t, err)
	return used
}

func TestReserveDeckQuota_ReservesUpToLimitThenReportsLimitReached(t *testing.T) {
	f := setup(t)
	owner := f.issuePlayer(t)
	limit := quotaLimit(t, 3)
	for range 3 {
		require.NoError(t, constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(owner), limit))
	}

	err := constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(owner), limit)

	require.ErrorIs(t, err, deck.ErrLimitReached)
	require.Equal(t, 3, f.used(t, owner))
}

func TestReserveDeckQuota_ReservesFirstWhenLimitIsOne(t *testing.T) {
	f := setup(t)
	owner := f.issuePlayer(t)
	limit := quotaLimit(t, 1)

	require.NoError(t, constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(owner), limit))

	require.ErrorIs(t, constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(owner), limit), deck.ErrLimitReached)
}

// 上限の値は文に渡すだけなので、変えても続きから数える
func TestReserveDeckQuota_FollowsChangedLimit(t *testing.T) {
	f := setup(t)
	owner := f.issuePlayer(t)
	for range 2 {
		require.NoError(t, constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(owner), quotaLimit(t, 2)))
	}
	require.ErrorIs(t, constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(owner), quotaLimit(t, 2)), deck.ErrLimitReached)

	require.NoError(t, constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(owner), quotaLimit(t, 3)))
	require.ErrorIs(t, constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(owner), quotaLimit(t, 1)), deck.ErrLimitReached)
	require.Equal(t, 3, f.used(t, owner))
}

func TestReserveDeckQuota_CountsPerOwner(t *testing.T) {
	f := setup(t)
	first, second := f.issuePlayer(t), f.issuePlayer(t)
	limit := quotaLimit(t, 1)
	require.NoError(t, constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(first), limit))

	require.NoError(t, constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(second), limit))
}

// 別の種別の数が上限に達していても、デッキの数には影響しない
func TestReserveDeckQuota_CountsPerQuotaType(t *testing.T) {
	f := setup(t)
	owner := f.issuePlayer(t)
	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	_, err = exec.ExecContext(f.ctx, `INSERT INTO domain_quota_types (id, name) VALUES (9900, 'constraint_test.other')`)
	require.NoError(t, err)
	_, err = exec.ExecContext(f.ctx, `INSERT INTO domain_quotas (quota_type_id, owner_id, used) VALUES (9900, $1, 5)`, owner.Int64())
	require.NoError(t, err)

	err = constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(owner), quotaLimit(t, 1))

	require.NoError(t, err)
}

func TestReleaseDeckQuota_DecreasesUsedAndStopsAtZero(t *testing.T) {
	f := setup(t)
	owner := f.issuePlayer(t)
	limit := quotaLimit(t, 2)
	require.NoError(t, constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(owner), limit))
	require.NoError(t, constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(owner), limit))

	require.NoError(t, constraint.ReleaseDeckQuota(f.ctx, deck.PlayerQuota(owner)))
	require.Equal(t, 1, f.used(t, owner))
	require.NoError(t, constraint.ReleaseDeckQuota(f.ctx, deck.PlayerQuota(owner)))
	require.NoError(t, constraint.ReleaseDeckQuota(f.ctx, deck.PlayerQuota(owner)))

	require.Equal(t, 0, f.used(t, owner))
	require.NoError(t, constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(owner), limit))
}

func TestReserveDeckQuota_DoesNotExceedLimitWhenReservedConcurrently(t *testing.T) {
	f := isolatedSetup(t)
	owner := f.issuePlayer(t)
	limit := quotaLimit(t, 5)
	var succeeded, reached atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := constraint.ReserveDeckQuota(f.ctx, deck.PlayerQuota(owner), limit)
			switch {
			case err == nil:
				succeeded.Add(1)
			case errors.Is(err, deck.ErrLimitReached):
				reached.Add(1)
			}
		}()
	}
	wg.Wait()

	require.Equal(t, int32(5), succeeded.Load())
	require.Equal(t, int32(15), reached.Load())
	require.Equal(t, 5, f.used(t, owner))
}

func TestUpdateDeckName_RejectsSameNormalizedNameOfSameOwnerOnly(t *testing.T) {
	f := setup(t)
	owner := f.issuePlayer(t)
	require.NoError(t, constraint.UpdateDeckName(f.ctx, f.issueDeck(t), owner, decktest.Name(t, "ABC")))

	err := constraint.UpdateDeckName(f.ctx, f.issueDeck(t), owner, decktest.Name(t, " ａｂｃ"))

	require.ErrorIs(t, err, deck.ErrNameAlreadyInUse)
	require.NoError(t, constraint.UpdateDeckName(f.ctx, f.issueDeck(t), f.issuePlayer(t), decktest.Name(t, "ABC")))
}
