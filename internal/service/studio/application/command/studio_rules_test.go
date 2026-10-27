package command_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
)

func countStudioRulesEvents(t *testing.T, ctx context.Context) int {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var count int
	require.NoError(t, exec.QueryRowContext(ctx, `SELECT count(*) FROM domain_events WHERE entity_type_id = 2400`).Scan(&count))
	return count
}

func TestCreateInitialStudioRules_CreatesDefaultsWhenMissing(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newStudioRulesHandler(t, ctx)

	created, err := handler.CreateInitial(ctx)

	require.NoError(t, err)
	require.True(t, created)
	got, err := handler.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, studiorules.Defaults(), got)
}

// 2 回目以降の起動で、何も増やさない
func TestCreateInitialStudioRules_DoesNothingWhenAlreadyCreated(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newStudioRulesHandler(t, ctx)
	_, err := handler.CreateInitial(ctx)
	require.NoError(t, err)

	created, err := handler.CreateInitial(ctx)

	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, 1, countStudioRulesEvents(t, ctx))
}

// 複数台が同時に起動しても、作るのは 1 つで、先を越されたほうもエラーにしない
func TestCreateInitialStudioRules_CreatesOnlyOnceWhenStartedConcurrently(t *testing.T) {
	ctx := isolatedDatabase(t)
	const starters = 5
	type result struct {
		created bool
		err     error
	}
	results := make(chan result, starters)
	for range starters {
		handler := newStudioRulesHandler(t, ctx)
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
	require.Equal(t, 1, countStudioRulesEvents(t, ctx))
}

func TestGetStudioRules_FailsBeforeCreated(t *testing.T) {
	ctx := isolatedDatabase(t)

	_, err := newStudioRulesHandler(t, ctx).Get(ctx)

	require.ErrorIs(t, err, command.ErrStudioRulesNotCreated)
}
