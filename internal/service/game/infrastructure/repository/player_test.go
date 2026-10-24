package repository_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
)

// 2 つの投影は別の集約から動くため、順番が決まらない。どちらが先でも同じ行（配布の状態が COMPLETED）になる
func TestPlayerWriters_ReachSameRowInEitherOrder(t *testing.T) {
	tests := []struct {
		name       string
		grantFirst bool
	}{
		{"player_then_grant", false},
		{"grant_then_player", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPlayerFixture(t)
			id, publicID := f.issuePlayer(t)
			players := repository.NewPlayerWriter()
			grants := repository.NewPlayerStarterGrantWriter(identifier.NewEntityRefResolver(f.registry))
			upsertPlayer := func() {
				require.NoError(t, players.Upsert(f.ctx, readmodel.Player{PlayerID: publicID}))
			}
			upsertGrant := func() {
				require.NoError(t, grants.Upsert(f.ctx, readmodel.PlayerStarterGrant{Player: id, Status: readmodel.StarterGrantCompleted}))
			}

			if tt.grantFirst {
				upsertGrant()
				upsertPlayer()
			} else {
				upsertPlayer()
				upsertGrant()
			}
			status, found, err := repository.NewPlayerReader().GetStarterGrantStatus(f.ctx, publicID)

			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, query.StarterGrantCompleted, status)
		})
	}
}

// 配布の途中の状態を、後から来る players の投影が消さない・状態の更新は上書きされる
func TestPlayerStarterGrantWriter_OverwritesOnlyStatus(t *testing.T) {
	f := newPlayerFixture(t)
	id, publicID := f.issuePlayer(t)
	grants := repository.NewPlayerStarterGrantWriter(identifier.NewEntityRefResolver(f.registry))
	reader := repository.NewPlayerReader()

	require.NoError(t, grants.Upsert(f.ctx, readmodel.PlayerStarterGrant{Player: id, Status: readmodel.StarterGrantInProgress}))
	require.NoError(t, repository.NewPlayerWriter().Upsert(f.ctx, readmodel.Player{PlayerID: publicID}))
	inProgress, found, err := reader.GetStarterGrantStatus(f.ctx, publicID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, query.StarterGrantInProgress, inProgress)

	require.NoError(t, grants.Upsert(f.ctx, readmodel.PlayerStarterGrant{Player: id, Status: readmodel.StarterGrantCompleted}))
	completed, _, err := reader.GetStarterGrantStatus(f.ctx, publicID)
	require.NoError(t, err)
	require.Equal(t, query.StarterGrantCompleted, completed)
}

func TestPlayerReader_ReturnsNotFoundWithoutStatus(t *testing.T) {
	f := newPlayerFixture(t)
	_, publicID := f.issuePlayer(t)
	reader := repository.NewPlayerReader()

	_, foundWithoutRow, err := reader.GetStarterGrantStatus(f.ctx, publicID)
	require.NoError(t, err)
	require.False(t, foundWithoutRow)

	require.NoError(t, repository.NewPlayerWriter().Upsert(f.ctx, readmodel.Player{PlayerID: publicID}))
	_, foundWithoutStatus, err := reader.GetStarterGrantStatus(f.ctx, publicID)
	require.NoError(t, err)
	require.False(t, foundWithoutStatus)
}
