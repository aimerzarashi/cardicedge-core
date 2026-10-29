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
func insertBoard(t *testing.T, ctx context.Context, sortName string, metadata readmodel.BoardMetadata) {
	t.Helper()
	raw, err := json.Marshal(metadata)
	require.NoError(t, err)
	insertBoardRow(t, ctx, metadata.ID, sortName, string(metadata.Status), raw)
}

func insertBoardRow(t *testing.T, ctx context.Context, id uuid.UUID, sortName, status string, metadata []byte) {
	t.Helper()
	row := models.Board{BoardID: id.String(), SortName: sortName, Status: status, Metadata: metadata}
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
}

func boardMetadata(name string, status readmodel.BoardStatus) readmodel.BoardMetadata {
	return readmodel.BoardMetadata{
		ID: uuid.New(), Name: name, Status: status, Revision: 3, Width: 3, Height: 2,
		Tiles: []readmodel.BoardTile{
			{X: 0, Y: 0, Kind: readmodel.BoardTileKindBase},
			{X: 1, Y: 0, Kind: readmodel.BoardTileKindLand, Attribute: readmodel.BoardTileAttributeWind},
			{X: 2, Y: 0, Kind: readmodel.BoardTileKindCheckpoint},
			{X: 0, Y: 1, Kind: readmodel.BoardTileKindWarp},
			{X: 1, Y: 1, Kind: readmodel.BoardTileKindShrine},
			{X: 2, Y: 1, Kind: readmodel.BoardTileKindWarp},
		},
		Connections:  []readmodel.BoardConnection{{A: readmodel.NewBoardPosition(0, 0), B: readmodel.NewBoardPosition(1, 0)}},
		Pairs:        []readmodel.BoardPair{{A: readmodel.NewBoardPosition(0, 1), B: readmodel.NewBoardPosition(2, 1)}},
		DefaultRules: readmodel.NewBoardDefaultRules(8000, 600, 300, 100),
		TileCounts:   readmodel.NewBoardTileCounts(1, 1, 1, 0, 0, 0, 1, 0, 0, 2, 0, 0, 1, 0, 0),
	}
}

func wantBoard(metadata readmodel.BoardMetadata) query.Board {
	connection := query.BoardConnection{A: query.NewBoardPosition(0, 0), B: query.NewBoardPosition(1, 0)}
	return query.Board{
		ID: metadata.ID, Name: metadata.Name, Width: 3, Height: 2,
		Tiles: []query.BoardTile{
			{X: 0, Y: 0, Kind: query.BoardTileKindBase},
			{X: 1, Y: 0, Kind: query.BoardTileKindLand, Attribute: query.BoardTileAttributeWind},
			{X: 2, Y: 0, Kind: query.BoardTileKindCheckpoint},
			{X: 0, Y: 1, Kind: query.BoardTileKindWarp},
			{X: 1, Y: 1, Kind: query.BoardTileKindShrine},
			{X: 2, Y: 1, Kind: query.BoardTileKindWarp},
		},
		Connections:  []query.BoardConnection{connection},
		Pairs:        []query.BoardPair{{A: query.NewBoardPosition(0, 1), B: query.NewBoardPosition(2, 1)}},
		DefaultRules: query.NewBoardDefaultRules(8000, 600, 300, 100),
		TileCounts:   query.NewBoardTileCounts(1, 1, 1, 0, 0, 0, 1, 0, 0, 2, 0, 0, 1, 0, 0),
	}
}

func TestBoardReader_ListsOnlyPublishedInSortNameOrder(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	b := boardMetadata("B", readmodel.BoardStatusPublished)
	a := boardMetadata("A", readmodel.BoardStatusPublished)
	insertBoard(t, ctx, "b", b)
	insertBoard(t, ctx, "a", a)
	insertBoard(t, ctx, "c", boardMetadata("C", readmodel.BoardStatusDraft))
	insertBoard(t, ctx, "d", boardMetadata("D", readmodel.BoardStatusRetired))

	got, err := repository.NewBoardReader().ListPublished(ctx, query.ListPublishedBoardsCondition{Limit: 10})

	require.NoError(t, err)
	cursorA := query.BoardListCursor{SortName: "a", BoardID: a.ID}
	cursorB := query.BoardListCursor{SortName: "b", BoardID: b.ID}
	require.Equal(t, []query.BoardListRow{
		{Board: wantBoard(a), Cursor: cursorA},
		{Board: wantBoard(b), Cursor: cursorB},
	}, got)
}

func TestBoardReader_ListContinuesAfterCursorUpToLimit(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	a, b, c := boardMetadata("A", readmodel.BoardStatusPublished), boardMetadata("B", readmodel.BoardStatusPublished), boardMetadata("C", readmodel.BoardStatusPublished)
	insertBoard(t, ctx, "a", a)
	insertBoard(t, ctx, "b", b)
	insertBoard(t, ctx, "c", c)

	after := query.BoardListCursor{SortName: "a", BoardID: a.ID}

	got, err := repository.NewBoardReader().ListPublished(ctx, query.ListPublishedBoardsCondition{After: &after, Limit: 1})

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, wantBoard(b), got[0].Board)
}

func TestBoardReader_ListReturnsEmptyWhenNothingIsPublished(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	insertBoard(t, ctx, "a", boardMetadata("A", readmodel.BoardStatusRetired))

	got, err := repository.NewBoardReader().ListPublished(ctx, query.ListPublishedBoardsCondition{Limit: 10})

	require.NoError(t, err)
	require.Empty(t, got)
}

func TestBoardReader_GetReturnsPublishedBoard(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	a := boardMetadata("A", readmodel.BoardStatusPublished)
	insertBoard(t, ctx, "a", a)

	got, err := repository.NewBoardReader().GetPublished(ctx, a.ID)

	require.NoError(t, err)
	require.Equal(t, wantBoard(a), got)
}

func TestBoardReader_GetReturnsNotFoundForBoardThatIsNotPublished(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	draft := boardMetadata("A", readmodel.BoardStatusDraft)
	retired := boardMetadata("B", readmodel.BoardStatusRetired)
	insertBoard(t, ctx, "a", draft)
	insertBoard(t, ctx, "b", retired)
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
			_, err := repository.NewBoardReader().GetPublished(ctx, tt.id)

			require.ErrorIs(t, err, query.ErrBoardNotFound)
		})
	}
}

// 知らない種類のマスを黙って捨てたり別の種類にしたりせず、読み出しを失敗させる（盤面が壊れて見えるより、気づけるようにする）
func TestBoardReader_GetFailsForUnknownTileKind(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	id := uuid.New()
	insertBoardRow(t, ctx, id, "a", "published", []byte(`{"name":"A","width":1,"height":1,"tiles":[{"x":0,"y":0,"kind":"water"}],"connections":[],"pairs":[],"tile_counts":`+completeTileCounts+`,"default_rules":{"target_magic":8000,"starting_magic":600,"lap_bonus":300,"checkpoint_bonus":100}}`))

	_, err := repository.NewBoardReader().GetPublished(ctx, id)

	require.Error(t, err)
}

func TestBoardReader_GetFailsForUnknownTileAttribute(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	id := uuid.New()
	insertBoardRow(t, ctx, id, "a", "published", []byte(`{"name":"A","width":1,"height":1,"tiles":[{"x":0,"y":0,"kind":"land","attribute":"light"}],"connections":[],"pairs":[],"tile_counts":`+completeTileCounts+`,"default_rules":{"target_magic":8000,"starting_magic":600,"lap_bonus":300,"checkpoint_bonus":100}}`))

	_, err := repository.NewBoardReader().GetPublished(ctx, id)

	require.Error(t, err)
}

// 既定のルールを写す前の行を、0 の値として読まず失敗させる（既定の値に黙って戻さない）
func TestBoardReader_GetFailsForBoardWithoutDefaultRules(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	id := uuid.New()
	insertBoardRow(t, ctx, id, "a", "published", []byte(`{"name":"A","width":1,"height":1,"tiles":[],"connections":[]}`))

	_, err := repository.NewBoardReader().GetPublished(ctx, id)

	require.Error(t, err)
}

func TestBoardReader_GetFailsForBoardWithoutTileCounts(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	id := uuid.New()
	insertBoardRow(t, ctx, id, "a", "published", []byte(`{"name":"A","width":1,"height":1,"tiles":[],"connections":[],"default_rules":{"target_magic":8000,"starting_magic":600,"lap_bonus":300,"checkpoint_bonus":100}}`))

	_, err := repository.NewBoardReader().GetPublished(ctx, id)

	require.Error(t, err)
}

// 特殊なマスの数を写す前の行（古い 9 項目の tile_counts）を、0 の値として読まず失敗させる
func TestBoardReader_GetFailsForBoardWithoutSpecialTileCounts(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	id := uuid.New()
	insertBoardRow(t, ctx, id, "a", "published", []byte(`{"name":"A","width":1,"height":1,"tiles":[],"connections":[],"pairs":[],"tile_counts":{"land":0,"base":0,"checkpoint":0,"fire":0,"earth":0,"water":0,"wind":0,"neutral":0,"multi":0},"default_rules":{"target_magic":8000,"starting_magic":600,"lap_bonus":300,"checkpoint_bonus":100}}`))

	_, err := repository.NewBoardReader().GetPublished(ctx, id)

	require.Error(t, err)
}

// 対を写す前の行を、対の無いボードとして読まず失敗させる
func TestBoardReader_GetFailsForBoardWithoutPairs(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	id := uuid.New()
	insertBoardRow(t, ctx, id, "a", "published", []byte(`{"name":"A","width":1,"height":1,"tiles":[],"connections":[],"tile_counts":`+completeTileCounts+`,"default_rules":{"target_magic":8000,"starting_magic":600,"lap_bonus":300,"checkpoint_bonus":100}}`))

	_, err := repository.NewBoardReader().GetPublished(ctx, id)

	require.Error(t, err)
}

const completeTileCounts = `{"land":0,"base":0,"checkpoint":0,"fire":0,"earth":0,"water":0,"wind":0,"neutral":0,"multi":0,"warp":0,"bridge":0,"sanctuary":0,"shrine":0,"prophecy_shop":0,"card_shop":0}`
