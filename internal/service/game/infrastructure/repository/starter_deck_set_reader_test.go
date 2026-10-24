package repository_test

import (
	"encoding/json"
	"testing"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

func TestStarterDeckSetReader_GetsPublishedStarterDeckSet(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	fireCard, waterCard := uuid.New(), uuid.New()
	// 公開は studio の read model の行をそのまま写すため、書き手の型で JSON を作り、読み手が同じ形を読めることを確かめる
	published, err := json.Marshal(readmodel.StarterDeckSetMetadata{
		Decks: []readmodel.StarterDeckSetDeck{
			{Name: "火", Cards: []readmodel.DeckCard{{CardID: fireCard, Count: 4}}},
			{Name: "水", Cards: []readmodel.DeckCard{{CardID: waterCard, Count: 3}, {CardID: fireCard, Count: 1}}},
		},
	})
	require.NoError(t, err)
	row := models.StarterDeckSet{StarterDeckSetID: uuid.NewString(), Metadata: published}
	require.NoError(t, row.Insert(ctx, db, boil.Infer()))

	got, found, err := repository.NewStarterDeckSetReader().Get(ctx)

	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, master.StarterDeckSet{Decks: []master.StarterDeck{
		{Name: "火", Cards: []master.DeckCard{{CardID: fireCard, Count: 4}}},
		{Name: "水", Cards: []master.DeckCard{{CardID: waterCard, Count: 3}, {CardID: fireCard, Count: 1}}},
	}}, got)
}

func TestStarterDeckSetReader_ReturnsNotFoundBeforePublished(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)

	_, found, err := repository.NewStarterDeckSetReader().Get(rdb.WithDB(t.Context(), db))

	require.NoError(t, err)
	require.False(t, found)
}
