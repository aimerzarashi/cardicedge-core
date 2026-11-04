package repository_test

import (
	"encoding/json"
	"testing"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

func TestGameRulesReader_GetsPublishedRules(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	want := gamerules.Defaults()
	want.Player.MaxDecks = playertest.MaxDecks(t, 3)
	// 公開は studio の read model の行をそのまま写すため、書き手の型で JSON を作り、読み手が同じ形を読めることを確かめる
	published, err := json.Marshal(readmodel.GameRulesMetadata{ID: uuid.New(), Revision: 1, Rules: want})
	require.NoError(t, err)
	row := models.GameRule{GameRulesID: uuid.NewString(), Metadata: published}
	require.NoError(t, row.Insert(ctx, db, boil.Infer()))

	got, found, err := repository.NewGameRulesReader().Get(ctx)

	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, want, got)
}

func TestGameRulesReader_ReturnsNotFoundBeforePublished(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)

	_, found, err := repository.NewGameRulesReader().Get(rdb.WithDB(t.Context(), db))

	require.NoError(t, err)
	require.False(t, found)
}
