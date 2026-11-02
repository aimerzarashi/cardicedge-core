package repository_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
)

// 行の metadata を、書き手の型で読み直して確かめる
func TestStudioRulesWriter_UpsertsRulesAsMetadata(t *testing.T) {
	ctx := emptyCardLists(t)
	publicID := uuid.New()
	at := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	rules := studiorules.Defaults()
	rules.Board.MaxSize = 16

	require.NoError(t, repository.NewStudioRulesWriter().Upsert(ctx, readmodel.StudioRules{StudioRulesID: publicID, Revision: 1, RegisteredAt: at, UpdatedAt: at, Rules: rules}))
	require.NoError(t, repository.NewStudioRulesWriter().Upsert(ctx, readmodel.StudioRules{StudioRulesID: publicID, Revision: 2, RegisteredAt: at, UpdatedAt: at, Rules: rules}))

	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	rows, err := models.StudioRules().All(ctx, exec)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, publicID.String(), rows[0].StudioRulesID)
	var metadata readmodel.StudioRulesMetadata
	require.NoError(t, json.Unmarshal(rows[0].Metadata, &metadata))
	require.Equal(t, readmodel.StudioRulesMetadata{ID: publicID, Revision: 2, RegisteredAt: at, UpdatedAt: at, Rules: rules}, metadata)
}

func TestStudioRulesReader_ReadsProjectedRules(t *testing.T) {
	ctx := emptyCardLists(t)
	rules := studiorules.Defaults()
	rules.Board.MaxSize = 16
	require.NoError(t, repository.NewStudioRulesWriter().Upsert(ctx, readmodel.StudioRules{StudioRulesID: uuid.New(), Revision: 1, UpdatedAt: time.Now().UTC(), Rules: rules}))

	got, err := repository.NewStudioRulesReader().Get(ctx)

	require.NoError(t, err)
	board := query.BoardRules{MinSize: 1, MaxSize: 16}
	require.Equal(t, query.StudioRules{Board: board}, got)
}

func TestStudioRulesReader_FailsBeforeProjected(t *testing.T) {
	ctx := emptyCardLists(t)

	_, err := repository.NewStudioRulesReader().Get(ctx)

	require.ErrorIs(t, err, query.ErrStudioRulesNotProjected)
}
