package repository_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
)

// 表の作りは書き手と読み手（game）の契約なので、行の metadata を、書き手の型で読み直して確かめる
func TestGameRulesWriter_UpsertsRulesAsMetadata(t *testing.T) {
	ctx := emptyCardLists(t)
	publicID := uuid.New()
	at := time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	rules := gamerules.Defaults()
	rules.Player.MaxDecks = 3

	require.NoError(t, repository.NewGameRulesWriter().Upsert(ctx, readmodel.GameRules{GameRulesID: publicID, Revision: 1, UpdatedAt: at, Rules: rules}))
	require.NoError(t, repository.NewGameRulesWriter().Upsert(ctx, readmodel.GameRules{GameRulesID: publicID, Revision: 2, UpdatedAt: at, Rules: rules}))

	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	rows, err := models.GameRules().All(ctx, exec)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, publicID.String(), rows[0].GameRulesID)
	var metadata readmodel.GameRulesMetadata
	require.NoError(t, json.Unmarshal(rows[0].Metadata, &metadata))
	require.Equal(t, readmodel.GameRulesMetadata{ID: publicID, Revision: 2, UpdatedAt: at, Rules: rules}, metadata)
}
