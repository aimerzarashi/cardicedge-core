package repository_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 解放の行を、投影が書く形（metadata の boards は解放した順の公開 ID）で直接入れる
func insertUnlock(t *testing.T, ctx context.Context, playerPublicID uuid.UUID, boards ...uuid.UUID) {
	t.Helper()
	id := uuid.New()
	metadata, err := json.Marshal(map[string]any{"id": id, "boards": boards})
	require.NoError(t, err)
	row := models.Unlock{UnlockID: id.String(), PlayerID: playerPublicID.String(), RegisteredAt: time.Now(), Metadata: metadata}
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
}

// 解放したボードの順に、公開中のものだけを読む。使用不可・準備中・写していないボードは飛ばし、ほかのプレイヤーの解放は混ざらない
func TestUnlockReader_ListsOwnUnlockedPublishedBoardsInUnlockOrder(t *testing.T) {
	f := newPlayerFixture(t)
	_, playerPublicID := f.issuePlayer(t)
	_, otherPublicID := f.issuePlayer(t)
	a := boardMetadata("A", readmodel.BoardStatusPublished)
	b := boardMetadata("B", readmodel.BoardStatusPublished)
	retired := boardMetadata("使用不可", readmodel.BoardStatusRetired)
	draft := boardMetadata("準備中", readmodel.BoardStatusDraft)
	for i, metadata := range []readmodel.BoardMetadata{a, b, retired, draft} {
		insertBoard(t, f.ctx, string(rune('a'+i)), metadata)
	}
	// 写していないボード（boards に行が無い）は飛ばす
	missing := uuid.New()
	insertUnlock(t, f.ctx, playerPublicID, b.ID, retired.ID, missing, draft.ID, a.ID)
	insertUnlock(t, f.ctx, otherPublicID, a.ID)

	got, err := repository.NewUnlockReader().ListBoardsByPlayer(f.ctx, playerPublicID)
	require.NoError(t, err)
	gotOther, err := repository.NewUnlockReader().ListBoardsByPlayer(f.ctx, otherPublicID)
	require.NoError(t, err)

	require.Equal(t, []string{"B", "A"}, boardNames(got))
	require.Equal(t, wantBoard(b), got[0])
	require.Equal(t, []string{"A"}, boardNames(gotOther))
}

func TestUnlockReader_ListsNothingForPlayerWithoutUnlock(t *testing.T) {
	f := newPlayerFixture(t)
	_, publicID := f.issuePlayer(t)

	got, err := repository.NewUnlockReader().ListBoardsByPlayer(f.ctx, publicID)

	require.NoError(t, err)
	require.Empty(t, got)
}

func boardNames(boards []query.Board) []string {
	names := make([]string, 0, len(boards))
	for _, board := range boards {
		names = append(names, board.Name)
	}
	return names
}
