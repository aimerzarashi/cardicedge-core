package repository_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 公開は studio の read model の行をそのまま写すため、書き手の型で JSON を作り、読み手が同じ形を読めることを確かめる
func insertStarterSet(t *testing.T, ctx context.Context, sortName string, status readmodel.StarterSetStatus, grantDecks, unlockBoards, unlockNPCAvatars []uuid.UUID) {
	t.Helper()
	id := uuid.New()
	metadata, err := json.Marshal(readmodel.StarterSetMetadata{ID: id, Name: sortName, Status: status, GrantDecks: grantDecks, UnlockBoards: unlockBoards, UnlockNPCAvatars: unlockNPCAvatars})
	require.NoError(t, err)
	row := models.StarterSet{StarterSetID: id.String(), SortName: sortName, Status: string(status), Metadata: metadata}
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
}

// 配るデッキはセットが持つ順（名前の順でも行の順でもない）で、公開中でないデッキは飛ばす。解放するボードはセットが持つ順のまま
func TestStarterSetReader_ReturnsGrantDecksInSetOrderSkippingUnpublishedAndUnlockBoards(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	card := uuid.New()
	fire, water, retired, missing := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	insertGrantDeck(t, ctx, fire, "a-fire", readmodel.GrantDeckStatusPublished, "火", readmodel.DeckCard{CardID: card, Count: 4})
	insertGrantDeck(t, ctx, water, "b-water", readmodel.GrantDeckStatusPublished, "水", readmodel.DeckCard{CardID: card, Count: 2})
	insertGrantDeck(t, ctx, retired, "c-retired", readmodel.GrantDeckStatusRetired, "使用不可")
	// 写したボードは、publish-master が game の domain_entities に行を作る（公開 ID は studio と同じ）
	boardB, boardBPublicID := issueBoardEntity(t, ctx)
	boardA, boardAPublicID := issueBoardEntity(t, ctx)
	npcA, npcAPublicID := issueNPCAvatarEntity(t, ctx)
	npcB, npcBPublicID := issueNPCAvatarEntity(t, ctx)
	insertStarterSet(t, ctx, "a", readmodel.StarterSetStatusPublished, []uuid.UUID{water, retired, missing, fire}, []uuid.UUID{boardBPublicID, boardAPublicID}, []uuid.UUID{npcAPublicID, npcBPublicID})

	got, found, err := newStarterSetReader(t, ctx).GetPublished(ctx)

	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, master.StarterSet{
		GrantDecks: []master.GrantDeck{
			{Name: "水", Cards: []master.DeckCard{{CardID: card, Count: 2}}},
			{Name: "火", Cards: []master.DeckCard{{CardID: card, Count: 4}}},
		},
		UnlockBoards:     []board.ID{boardB, boardA},
		UnlockNPCAvatars: []npcavatar.ID{npcA, npcB},
	}, got)
}

func TestStarterSetReader_ReadsOnlyPublishedSet(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	fire, other := uuid.New(), uuid.New()
	insertGrantDeck(t, ctx, fire, "a-fire", readmodel.GrantDeckStatusPublished, "火")
	insertGrantDeck(t, ctx, other, "b-other", readmodel.GrantDeckStatusPublished, "他")
	insertStarterSet(t, ctx, "a-draft", readmodel.StarterSetStatusDraft, []uuid.UUID{other}, nil, nil)
	insertStarterSet(t, ctx, "b-retired", readmodel.StarterSetStatusRetired, []uuid.UUID{other}, nil, nil)
	insertStarterSet(t, ctx, "c-published", readmodel.StarterSetStatusPublished, []uuid.UUID{fire}, nil, nil)

	got, found, err := newStarterSetReader(t, ctx).GetPublished(ctx)

	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, []master.GrantDeck{{Name: "火", Cards: []master.DeckCard{}}}, got.GrantDecks)
	require.Empty(t, got.UnlockBoards)
	require.NotNil(t, got.UnlockBoards)
}

// 写していないボード（game の domain_entities に行が無い）を解放するセットは、黙って捨てず失敗にする（publish-master を流せば直る）
func TestStarterSetReader_FailsForUnlockBoardThatWasNotPublishedToGame(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	fire := uuid.New()
	insertGrantDeck(t, ctx, fire, "a-fire", readmodel.GrantDeckStatusPublished, "火")
	insertStarterSet(t, ctx, "a", readmodel.StarterSetStatusPublished, []uuid.UUID{fire}, []uuid.UUID{uuid.New()}, nil)

	_, _, err := newStarterSetReader(t, ctx).GetPublished(ctx)

	require.ErrorIs(t, err, domain.ErrEntityNotFound)
}

func newStarterSetReader(t *testing.T, ctx context.Context) *repository.StarterSetReader {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return repository.NewStarterSetReader(repository.NewGrantDeckReader(), identifier.New(registry, board.NewID), identifier.New(registry, npcavatar.NewID))
}

func issueBoardEntity(t *testing.T, ctx context.Context) (board.ID, uuid.UUID) {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, publicID, err := identifier.New(registry, board.NewID).Issue(ctx)
	require.NoError(t, err)
	return id, publicID
}

// 写していない NPC のアバター（game の domain_entities に行が無い）も同じく失敗にする
func TestStarterSetReader_FailsForUnlockNPCAvatarThatWasNotPublishedToGame(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	fire := uuid.New()
	insertGrantDeck(t, ctx, fire, "a-fire", readmodel.GrantDeckStatusPublished, "火")
	insertStarterSet(t, ctx, "a", readmodel.StarterSetStatusPublished, []uuid.UUID{fire}, nil, []uuid.UUID{uuid.New()})

	_, _, err := newStarterSetReader(t, ctx).GetPublished(ctx)

	require.ErrorIs(t, err, domain.ErrEntityNotFound)
}

func issueNPCAvatarEntity(t *testing.T, ctx context.Context) (npcavatar.ID, uuid.UUID) {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, publicID, err := identifier.New(registry, npcavatar.NewID).Issue(ctx)
	require.NoError(t, err)
	return id, publicID
}

func TestStarterSetReader_ReportsNotFoundWhenNothingIsPublished(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	insertStarterSet(t, ctx, "a-draft", readmodel.StarterSetStatusDraft, nil, nil, nil)
	insertStarterSet(t, ctx, "b-retired", readmodel.StarterSetStatusRetired, nil, nil, nil)

	got, found, err := newStarterSetReader(t, ctx).GetPublished(ctx)

	require.NoError(t, err)
	require.False(t, found)
	require.Empty(t, got)
}
