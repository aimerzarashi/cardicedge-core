package command_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
)

func countGameRulesEvents(t *testing.T, ctx context.Context) int {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var count int
	require.NoError(t, exec.QueryRowContext(ctx, `SELECT count(*) FROM domain_events WHERE entity_type_id = 2000`).Scan(&count))
	return count
}

func TestCreateInitialGameRules_CreatesDefaultsWhenMissing(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newGameRulesHandler(t, ctx)

	created, err := handler.CreateInitial(ctx)

	require.NoError(t, err)
	require.True(t, created)
	got, err := handler.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, gamerules.Defaults(), got)
}

// 2 回目以降の起動で、何も増やさない
func TestCreateInitialGameRules_DoesNothingWhenAlreadyCreated(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newGameRulesHandler(t, ctx)
	_, err := handler.CreateInitial(ctx)
	require.NoError(t, err)

	created, err := handler.CreateInitial(ctx)

	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, 1, countGameRulesEvents(t, ctx))
}

// 複数台が同時に起動しても、作るのは 1 つで、先を越されたほうもエラーにしない
func TestCreateInitialGameRules_CreatesOnlyOnceWhenStartedConcurrently(t *testing.T) {
	ctx := isolatedDatabase(t)
	const starters = 5
	type result struct {
		created bool
		err     error
	}
	results := make(chan result, starters)
	for range starters {
		handler := newGameRulesHandler(t, ctx)
		go func() {
			created, err := handler.CreateInitial(ctx)
			results <- result{created: created, err: err}
		}()
	}

	created := 0
	for range starters {
		r := <-results
		require.NoError(t, r.err)
		if r.created {
			created++
		}
	}

	require.Equal(t, 1, created)
	require.Equal(t, 1, countGameRulesEvents(t, ctx))
}

func TestGetGameRules_FailsBeforeCreated(t *testing.T) {
	ctx := isolatedDatabase(t)

	_, err := newGameRulesHandler(t, ctx).Get(ctx)

	require.ErrorIs(t, err, command.ErrGameRulesNotCreated)
}
