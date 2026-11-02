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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 公開は studio の read model の行をそのまま写すため、書き手の型で JSON を作り、読み手が同じ形を読めることを確かめる
func insertNPCAvatar(t *testing.T, ctx context.Context, sortName string, metadata readmodel.NPCAvatarMetadata) {
	t.Helper()
	raw, err := json.Marshal(metadata)
	require.NoError(t, err)
	insertNPCAvatarRow(t, ctx, metadata.ID, sortName, string(metadata.Status), raw)
}

func insertNPCAvatarRow(t *testing.T, ctx context.Context, id uuid.UUID, sortName, status string, metadata []byte) {
	t.Helper()
	row := models.NPCAvatar{NPCAvatarID: id.String(), SortName: sortName, Status: status, Metadata: metadata}
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
}

func npcAvatarMetadata(name string, status readmodel.NPCAvatarStatus, decks ...uuid.UUID) readmodel.NPCAvatarMetadata {
	return readmodel.NPCAvatarMetadata{ID: uuid.New(), Name: name, Status: status, Revision: 3, Decks: append([]uuid.UUID{}, decks...)}
}

func insertNPCDeck(t *testing.T, ctx context.Context, name string, status readmodel.NPCDeckStatus, cards ...readmodel.DeckCard) uuid.UUID {
	t.Helper()
	metadata := readmodel.NPCDeckMetadata{ID: uuid.New(), Name: name, Status: status, Revision: 2, Cards: cards}
	raw, err := json.Marshal(metadata)
	require.NoError(t, err)
	row := models.NPCDeck{NPCDeckID: metadata.ID.String(), SortName: name, Status: string(status), Metadata: raw}
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
	return metadata.ID
}

func insertNPCDeckCard(t *testing.T, ctx context.Context, name string, cardType readmodel.CardType) uuid.UUID {
	t.Helper()
	id := uuid.New()
	raw, err := json.Marshal(readmodel.CardMetadata{ID: id, Name: name, Status: readmodel.CardStatusPublished})
	require.NoError(t, err)
	row := models.Card{CardID: id.String(), CardType: int16(cardType), SortName: name, Status: string(readmodel.CardStatusPublished), Metadata: raw}
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
	return id
}

func TestNPCAvatarReader_ListsOnlyPublishedInSortNameOrderWithDeckIDsInAssignedOrder(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	d1, d2 := uuid.New(), uuid.New()
	b := npcAvatarMetadata("B", readmodel.NPCAvatarStatusPublished)
	a := npcAvatarMetadata("A", readmodel.NPCAvatarStatusPublished, d2, d1)
	insertNPCAvatar(t, ctx, "b", b)
	insertNPCAvatar(t, ctx, "a", a)
	insertNPCAvatar(t, ctx, "c", npcAvatarMetadata("C", readmodel.NPCAvatarStatusDraft))
	insertNPCAvatar(t, ctx, "d", npcAvatarMetadata("D", readmodel.NPCAvatarStatusRetired))

	got, err := repository.NewNPCAvatarReader().ListPublished(ctx, query.ListPublishedNPCAvatarsCondition{Limit: 10})

	require.NoError(t, err)
	wantA := query.NPCAvatarRecord{ID: a.ID, Name: "A", DeckIDs: []uuid.UUID{d2, d1}}
	wantB := query.NPCAvatarRecord{ID: b.ID, Name: "B", DeckIDs: []uuid.UUID{}}
	cursorA := query.NPCAvatarListCursor{SortName: "a", NPCAvatarID: a.ID}
	cursorB := query.NPCAvatarListCursor{SortName: "b", NPCAvatarID: b.ID}
	require.Equal(t, []query.NPCAvatarListRow{{Avatar: wantA, Cursor: cursorA}, {Avatar: wantB, Cursor: cursorB}}, got)
}

func TestNPCAvatarReader_ListContinuesAfterCursorUpToLimit(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	a, b, c := npcAvatarMetadata("A", readmodel.NPCAvatarStatusPublished), npcAvatarMetadata("B", readmodel.NPCAvatarStatusPublished), npcAvatarMetadata("C", readmodel.NPCAvatarStatusPublished)
	insertNPCAvatar(t, ctx, "a", a)
	insertNPCAvatar(t, ctx, "b", b)
	insertNPCAvatar(t, ctx, "c", c)
	after := query.NPCAvatarListCursor{SortName: "a", NPCAvatarID: a.ID}

	got, err := repository.NewNPCAvatarReader().ListPublished(ctx, query.ListPublishedNPCAvatarsCondition{After: &after, Limit: 1})

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, b.ID, got[0].Avatar.ID)
}

func TestNPCAvatarReader_ListReturnsEmptyWhenNothingIsPublished(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	insertNPCAvatar(t, ctx, "a", npcAvatarMetadata("A", readmodel.NPCAvatarStatusRetired))

	got, err := repository.NewNPCAvatarReader().ListPublished(ctx, query.ListPublishedNPCAvatarsCondition{Limit: 10})

	require.NoError(t, err)
	require.Empty(t, got)
}

func TestNPCAvatarReader_GetReturnsPublishedAvatar(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	deck := uuid.New()
	a := npcAvatarMetadata("A", readmodel.NPCAvatarStatusPublished, deck)
	insertNPCAvatar(t, ctx, "a", a)

	got, err := repository.NewNPCAvatarReader().GetPublished(ctx, a.ID)

	require.NoError(t, err)
	require.Equal(t, query.NPCAvatarRecord{ID: a.ID, Name: "A", DeckIDs: []uuid.UUID{deck}}, got)
}

func TestNPCAvatarReader_GetReturnsNotFoundForAvatarThatIsNotPublished(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	draft := npcAvatarMetadata("A", readmodel.NPCAvatarStatusDraft)
	retired := npcAvatarMetadata("B", readmodel.NPCAvatarStatusRetired)
	insertNPCAvatar(t, ctx, "a", draft)
	insertNPCAvatar(t, ctx, "b", retired)
	tests := []struct {
		name string
		id   uuid.UUID
	}{
		{name: "draft", id: draft.ID},
		{name: "retired", id: retired.ID},
		{name: "unknown", id: uuid.New()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repository.NewNPCAvatarReader().GetPublished(ctx, tt.id)

			require.ErrorIs(t, err, query.ErrNPCAvatarNotFound)
		})
	}
}

// decks が無い行（デッキを持たせる前に写した行）を、デッキが無いものとして返さず、読み出しを失敗させる
func TestNPCAvatarReader_GetFailsWhenDecksIsMissing(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	id := uuid.New()
	insertNPCAvatarRow(t, ctx, id, "a", "published", []byte(`{"name":"A"}`))

	_, err := repository.NewNPCAvatarReader().GetPublished(ctx, id)

	require.ErrorContains(t, err, "decks is missing")
}

func TestNPCAvatarReader_ListDecksReturnsOnlyPublishedRequestedDecksWithCardsInTypeThenNameOrder(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	spell := insertNPCDeckCard(t, ctx, "a", readmodel.CardTypeSpell)
	itemB := insertNPCDeckCard(t, ctx, "b", readmodel.CardTypeItem)
	itemA := insertNPCDeckCard(t, ctx, "a", readmodel.CardTypeItem)
	creature := insertNPCDeckCard(t, ctx, "z", readmodel.CardTypeCreature)
	published := insertNPCDeck(t, ctx, "公開中", readmodel.NPCDeckStatusPublished,
		readmodel.DeckCard{CardID: spell, Count: 1}, readmodel.DeckCard{CardID: itemB, Count: 2},
		readmodel.DeckCard{CardID: creature, Count: 3}, readmodel.DeckCard{CardID: itemA, Count: 4})
	draft := insertNPCDeck(t, ctx, "準備中", readmodel.NPCDeckStatusDraft, readmodel.DeckCard{CardID: creature, Count: 1})
	retired := insertNPCDeck(t, ctx, "使用不可", readmodel.NPCDeckStatusRetired, readmodel.DeckCard{CardID: creature, Count: 1})
	insertNPCDeck(t, ctx, "頼まれていない", readmodel.NPCDeckStatusPublished, readmodel.DeckCard{CardID: creature, Count: 1})

	got, err := repository.NewNPCAvatarReader().ListPublishedDecks(ctx, []uuid.UUID{published, draft, retired, uuid.New()})

	require.NoError(t, err)
	require.Equal(t, []query.NPCDeck{{
		ID: published, Name: "公開中",
		Cards: []query.NPCDeckCard{
			{CardID: creature, Name: "z", Type: query.CardTypeCreature, Count: 3},
			{CardID: itemA, Name: "a", Type: query.CardTypeItem, Count: 4},
			{CardID: itemB, Name: "b", Type: query.CardTypeItem, Count: 2},
			{CardID: spell, Name: "a", Type: query.CardTypeSpell, Count: 1},
		},
	}}, got)
}

func TestNPCAvatarReader_ListDecksReturnsNothingForNoIDs(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)

	got, err := repository.NewNPCAvatarReader().ListPublishedDecks(ctx, nil)

	require.NoError(t, err)
	require.Empty(t, got)
}

// 写していないカードを黙って捨てず、読み出しを失敗させる（枚数の合計が実際と食い違って見えるより、気づけるようにする）
func TestNPCAvatarReader_ListDecksFailsForCardThatWasNotPublishedToGame(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	deck := insertNPCDeck(t, ctx, "公開中", readmodel.NPCDeckStatusPublished, readmodel.DeckCard{CardID: uuid.New(), Count: 1})

	_, err := repository.NewNPCAvatarReader().ListPublishedDecks(ctx, []uuid.UUID{deck})

	require.ErrorContains(t, err, "is not published to game")
}
