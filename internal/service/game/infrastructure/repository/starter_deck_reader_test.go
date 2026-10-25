package repository_test

import (
	"context"
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

// 公開は studio の read model の行をそのまま写すため、書き手の型で JSON を作り、読み手が同じ形を読めることを確かめる
func insertStarterDeck(t *testing.T, ctx context.Context, id uuid.UUID, sortName string, status readmodel.StarterDeckStatus, name string, cards ...readmodel.DeckCard) {
	t.Helper()
	metadata, err := json.Marshal(readmodel.StarterDeckMetadata{ID: id, Name: name, Status: status, Cards: cards})
	require.NoError(t, err)
	row := models.StarterDeck{StarterDeckID: id.String(), SortName: sortName, Status: string(status), Metadata: metadata}
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
}

func TestStarterDeckReader_ListsOnlyPublishedInSortNameOrder(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	fireCard, waterCard := uuid.New(), uuid.New()
	insertStarterDeck(t, ctx, uuid.New(), "b-water", readmodel.StarterDeckStatusPublished, "水", readmodel.DeckCard{CardID: waterCard, Count: 3}, readmodel.DeckCard{CardID: fireCard, Count: 1})
	insertStarterDeck(t, ctx, uuid.New(), "a-fire", readmodel.StarterDeckStatusPublished, "火", readmodel.DeckCard{CardID: fireCard, Count: 4})
	insertStarterDeck(t, ctx, uuid.New(), "c-draft", readmodel.StarterDeckStatusDraft, "準備中")
	insertStarterDeck(t, ctx, uuid.New(), "d-retired", readmodel.StarterDeckStatusRetired, "使用不可")

	got, err := repository.NewStarterDeckReader().ListPublished(ctx)

	require.NoError(t, err)
	require.Equal(t, []master.StarterDeck{
		{Name: "火", Cards: []master.DeckCard{{CardID: fireCard, Count: 4}}},
		{Name: "水", Cards: []master.DeckCard{{CardID: waterCard, Count: 3}, {CardID: fireCard, Count: 1}}},
	}, got)
}

func TestStarterDeckReader_ReturnsEmptyWhenNothingIsPublished(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	insertStarterDeck(t, ctx, uuid.New(), "a-draft", readmodel.StarterDeckStatusDraft, "準備中")

	got, err := repository.NewStarterDeckReader().ListPublished(ctx)

	require.NoError(t, err)
	require.Empty(t, got)
}
