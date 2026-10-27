package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
)

type boardFixture struct {
	presetDeckFixture
	writer *repository.BoardWriter
}

func newBoardFixture(t *testing.T) boardFixture {
	t.Helper()
	return boardFixture{presetDeckFixture: newPresetDeckFixture(t), writer: repository.NewBoardWriter()}
}

func boardState(sortName, name string, status readmodel.BoardStatus) readmodel.Board {
	id := uuid.New()
	at := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	metadata := readmodel.BoardMetadata{
		ID: id, Name: name, Status: status, Revision: 3, Width: 8, Height: 8,
		Tiles: []readmodel.BoardTile{}, Connections: []readmodel.BoardConnection{}, RegisteredAt: at, UpdatedAt: at,
	}
	return readmodel.Board{BoardID: id, SortName: sortName, Status: status, Metadata: metadata}
}

func (f boardFixture) put(t *testing.T, sortName, name string, status readmodel.BoardStatus) readmodel.Board {
	t.Helper()
	board := boardState(sortName, name, status)
	require.NoError(t, f.writer.Upsert(f.ctx, board))
	return board
}

func (f boardFixture) list(t *testing.T, condition query.ListBoardsCondition) []query.BoardListRow {
	t.Helper()
	rows, err := repository.NewBoardReader().List(f.ctx, condition)
	require.NoError(t, err)
	return rows
}

func (f boardFixture) listAll(t *testing.T) []query.BoardListRow {
	t.Helper()
	return f.list(t, query.ListBoardsCondition{Limit: 100})
}

func boardNames(rows []query.BoardListRow) []string {
	result := make([]string, len(rows))
	for i, row := range rows {
		result[i] = row.Board.Name
	}
	return result
}

// 大きさ・マス・つながりは metadata に入れ、読むときに並びも含めてそのまま返る
func TestBoard_GetsUpsertedBoardWithLayout(t *testing.T) {
	f := newBoardFixture(t)
	board := boardState("fire", "Ｆire", readmodel.BoardStatusPublished)
	board.Metadata.Width, board.Metadata.Height = 4, 3
	board.Metadata.Tiles = []readmodel.BoardTile{{X: 0, Y: 0, Kind: readmodel.BoardTileKindBase}, {X: 1, Y: 0, Kind: readmodel.BoardTileKindCheckpoint}}
	board.Metadata.Connections = []readmodel.BoardConnection{{A: readmodel.NewBoardPosition(0, 0), B: readmodel.NewBoardPosition(1, 0)}}

	require.NoError(t, f.writer.Upsert(f.ctx, board))
	got, err := repository.NewBoardReader().Get(f.ctx, board.BoardID)

	require.NoError(t, err)
	require.Equal(t, board.BoardID, got.ID)
	require.Equal(t, "Ｆire", got.Name)
	require.Equal(t, readmodel.BoardStatusPublished, got.Status)
	require.Equal(t, 4, got.Width)
	require.Equal(t, 3, got.Height)
	require.Equal(t, board.Metadata.Tiles, got.Tiles)
	require.Equal(t, board.Metadata.Connections, got.Connections)
	require.Equal(t, int64(3), got.Revision)
	require.Equal(t, board.Metadata.RegisteredAt, got.RegisteredAt.UTC())
	require.Equal(t, board.Metadata.UpdatedAt, got.UpdatedAt.UTC())
}

func TestBoard_GetFailsForMissingBoard(t *testing.T) {
	f := newBoardFixture(t)

	_, err := repository.NewBoardReader().Get(f.ctx, uuid.New())

	require.ErrorIs(t, err, query.ErrBoardNotFound)
}

func TestBoard_UpsertOverwritesSameBoard(t *testing.T) {
	f := newBoardFixture(t)
	board := f.put(t, "fire", "Fire", readmodel.BoardStatusDraft)
	board.SortName = "water"
	board.Metadata.Name = "Water"
	board.Status = readmodel.BoardStatusRetired
	board.Metadata.Status = readmodel.BoardStatusRetired

	require.NoError(t, f.writer.Upsert(f.ctx, board))

	rows := f.listAll(t)
	require.Equal(t, []string{"Water"}, boardNames(rows))
	require.Equal(t, readmodel.BoardStatusRetired, rows[0].Board.Status)
}

func TestBoard_DeleteRemovesBoardAndAcceptsMissingOne(t *testing.T) {
	f := newBoardFixture(t)
	deleted := f.put(t, "fire", "Fire", readmodel.BoardStatusDraft)
	f.put(t, "water", "Water", readmodel.BoardStatusDraft)

	require.NoError(t, f.writer.Delete(f.ctx, deleted))
	require.NoError(t, f.writer.Delete(f.ctx, deleted))

	require.Equal(t, []string{"Water"}, boardNames(f.listAll(t)))
}

func TestBoard_ListsBySortNameWithCursor(t *testing.T) {
	f := newBoardFixture(t)
	f.put(t, "b", "B", readmodel.BoardStatusDraft)
	first := f.put(t, "a", "A", readmodel.BoardStatusDraft)
	f.put(t, "c", "C", readmodel.BoardStatusDraft)
	after := query.BoardListCursor{SortName: first.SortName, BoardID: first.BoardID}

	rows := f.list(t, query.ListBoardsCondition{After: &after, Limit: 1})

	require.Equal(t, []string{"A", "B", "C"}, boardNames(f.listAll(t)))
	require.Equal(t, []string{"B"}, boardNames(rows))
	require.Equal(t, query.BoardListCursor{SortName: "b", BoardID: rows[0].Board.ID}, rows[0].Cursor)
}

func TestBoard_FiltersByStatuses(t *testing.T) {
	f := newBoardFixture(t)
	f.put(t, "a", "A", readmodel.BoardStatusDraft)
	f.put(t, "b", "B", readmodel.BoardStatusPublished)
	f.put(t, "c", "C", readmodel.BoardStatusRetired)

	published := f.list(t, query.ListBoardsCondition{Statuses: []readmodel.BoardStatus{readmodel.BoardStatusPublished}, Limit: 10})
	notDraft := f.list(t, query.ListBoardsCondition{
		Statuses: []readmodel.BoardStatus{readmodel.BoardStatusPublished, readmodel.BoardStatusRetired}, Limit: 10,
	})

	require.Equal(t, []string{"B"}, boardNames(published))
	require.Equal(t, []string{"B", "C"}, boardNames(notDraft))
}

// LIKE の記号（% _）は文字どおりに比べる（カードの一覧と同じ）
func TestBoard_FiltersByNameContainsLiterally(t *testing.T) {
	f := newBoardFixture(t)
	for _, name := range []string{"50%off", "50xoff"} {
		f.put(t, name, name, readmodel.BoardStatusDraft)
	}

	rows := f.list(t, query.ListBoardsCondition{NameContains: "50%", Limit: 10})

	require.Equal(t, []string{"50%off"}, boardNames(rows))
}
