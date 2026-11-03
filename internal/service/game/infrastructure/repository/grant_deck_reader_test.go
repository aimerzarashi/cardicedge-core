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
func insertGrantDeck(t *testing.T, ctx context.Context, id uuid.UUID, sortName string, status readmodel.GrantDeckStatus, name string, cards ...readmodel.DeckCard) {
	t.Helper()
	metadata, err := json.Marshal(readmodel.GrantDeckMetadata{ID: id, Name: name, Status: status, Cards: cards})
	require.NoError(t, err)
	row := models.GrantDeck{GrantDeckID: id.String(), SortName: sortName, Status: string(status), Metadata: metadata}
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
}

func TestGrantDeckReader_ListsOnlyPublishedInSortNameOrder(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	fireCard, waterCard := uuid.New(), uuid.New()
	insertGrantDeck(t, ctx, uuid.New(), "b-water", readmodel.GrantDeckStatusPublished, "水", readmodel.DeckCard{CardID: waterCard, Count: 3}, readmodel.DeckCard{CardID: fireCard, Count: 1})
	insertGrantDeck(t, ctx, uuid.New(), "a-fire", readmodel.GrantDeckStatusPublished, "火", readmodel.DeckCard{CardID: fireCard, Count: 4})
	insertGrantDeck(t, ctx, uuid.New(), "c-draft", readmodel.GrantDeckStatusDraft, "準備中")
	insertGrantDeck(t, ctx, uuid.New(), "d-retired", readmodel.GrantDeckStatusRetired, "使用不可")

	got, err := repository.NewGrantDeckReader().ListPublished(ctx)

	require.NoError(t, err)
	require.Equal(t, []master.GrantDeck{
		{Name: "火", Cards: []master.DeckCard{{CardID: fireCard, Count: 4}}},
		{Name: "水", Cards: []master.DeckCard{{CardID: waterCard, Count: 3}, {CardID: fireCard, Count: 1}}},
	}, got)
}

func TestGrantDeckReader_ReturnsEmptyWhenNothingIsPublished(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	insertGrantDeck(t, ctx, uuid.New(), "a-draft", readmodel.GrantDeckStatusDraft, "準備中")

	got, err := repository.NewGrantDeckReader().ListPublished(ctx)

	require.NoError(t, err)
	require.Empty(t, got)
}
