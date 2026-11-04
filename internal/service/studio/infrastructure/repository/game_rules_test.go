package repository_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules/gamerulestest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
)

// 表の作りは書き手と読み手（game）の契約なので、行の metadata を、書き手の型で読み直して確かめる
func TestGameRulesWriter_UpsertsRulesAsMetadata(t *testing.T) {
	ctx := emptyCardLists(t)
	publicID := uuid.New()
	at := time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	rules := gamerules.Defaults()
	rules.Player.MaxDecks = playertest.MaxDecks(t, 3)
	rules.Match.MaxHandCards = gamerulestest.MaxHandCards(t, 8)

	require.NoError(t, repository.NewGameRulesWriter().Upsert(ctx, readmodel.GameRules{GameRulesID: publicID, Revision: 1, RegisteredAt: at, UpdatedAt: at, Rules: rules}))
	require.NoError(t, repository.NewGameRulesWriter().Upsert(ctx, readmodel.GameRules{GameRulesID: publicID, Revision: 2, RegisteredAt: at, UpdatedAt: at, Rules: rules}))

	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	rows, err := models.GameRules().All(ctx, exec)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, publicID.String(), rows[0].GameRulesID)
	var metadata readmodel.GameRulesMetadata
	require.NoError(t, json.Unmarshal(rows[0].Metadata, &metadata))
	require.Equal(t, readmodel.GameRulesMetadata{ID: publicID, Revision: 2, RegisteredAt: at, UpdatedAt: at, Rules: rules}, metadata)
}

func TestGameRulesReader_ReadsProjectedMatchRules(t *testing.T) {
	ctx := emptyCardLists(t)
	rules := gamerules.Defaults()
	rules.Match = gamerulestest.Match(t, 3, 5, 8)
	require.NoError(t, repository.NewGameRulesWriter().Upsert(ctx, readmodel.GameRules{GameRulesID: uuid.New(), Revision: 1, UpdatedAt: time.Now().UTC(), Rules: rules}))

	got, err := repository.NewGameRulesReader().Get(ctx)

	require.NoError(t, err)
	match := query.MatchRules{MinPlayers: 3, MaxPlayers: 5, MaxHandCards: 8}
	require.Equal(t, query.GameRules{Match: match}, got)
}

func TestGameRulesReader_FailsBeforeProjected(t *testing.T) {
	ctx := emptyCardLists(t)

	_, err := repository.NewGameRulesReader().Get(ctx)

	require.ErrorIs(t, err, query.ErrGameRulesNotProjected)
}
