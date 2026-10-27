package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

var (
	boardRegisteredAt = time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	boardChangedAt    = time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
)

// 集約の Decide・Evolve でコマンドを順に当て、そのイベントを read model に当てる
type boardHistory struct {
	t      *testing.T
	state  board.State
	events []board.Event
}

func newBoardHistory(t *testing.T, publicID uuid.UUID, name string) *boardHistory {
	t.Helper()
	h := &boardHistory{t: t, state: board.InitialState()}
	h.apply(&board.RegisterCommand{ID: boardtest.ID(t, 1), PublicID: publicID, Name: boardtest.Name(t, name), Rules: studiorules.Defaults().Board, RegisteredBy: operatortest.ID(t, 10)}, boardRegisteredAt)
	return h
}

func (h *boardHistory) apply(cmd board.Command, at time.Time) {
	h.t.Helper()
	events, err := board.Decide(h.state, cmd, at)
	require.NoError(h.t, err)
	for _, event := range events {
		h.state, err = board.Evolve(h.state, event)
		require.NoError(h.t, err)
	}
	h.events = append(h.events, events...)
}

func (h *boardHistory) changeLayout(layout board.Layout) {
	h.t.Helper()
	h.apply(&board.ChangeLayoutCommand{Layout: layout, Rules: studiorules.Defaults().Board, ChangedBy: operatortest.ID(h.t, 10)}, boardChangedAt)
}

func (h *boardHistory) project() readmodel.Board {
	var state readmodel.Board
	for _, event := range h.events {
		state = state.Evolve(event)
	}
	return state
}

func TestBoard_EvolvesFromRegistered(t *testing.T) {
	publicID := uuid.New()

	state := newBoardHistory(t, publicID, "Ｆire 草原").project()

	require.Equal(t, publicID, state.BoardID)
	require.Equal(t, "fire 草原", state.SortName)
	require.Equal(t, readmodel.BoardStatusDraft, state.Status)
	require.Equal(t, readmodel.BoardMetadata{
		ID: publicID, Name: "Ｆire 草原", Status: readmodel.BoardStatusDraft, Revision: 1, Width: 8, Height: 8,
		Tiles: []readmodel.BoardTile{}, Connections: []readmodel.BoardConnection{},
		RegisteredAt: boardRegisteredAt, UpdatedAt: boardRegisteredAt,
	}, state.Metadata)
	require.False(t, state.IsTerminal())
}

func TestBoard_RenameChangesNameAndSortName(t *testing.T) {
	h := newBoardHistory(t, uuid.New(), "草原")
	h.apply(&board.RenameCommand{Name: boardtest.Name(t, "Ｓand"), RenamedBy: operatortest.ID(t, 10)}, boardChangedAt)

	state := h.project()

	require.Equal(t, "sand", state.SortName)
	require.Equal(t, "Ｓand", state.Metadata.Name)
	require.Equal(t, int64(2), state.Metadata.Revision)
	require.Equal(t, boardChangedAt, state.Metadata.UpdatedAt)
}

// マスは y → x、つながりは a → b の順に並べる。差分を当てた結果が、入れた順によらず同じ並びになる
func TestBoard_LayoutChangeAppliesDifferenceInOrder(t *testing.T) {
	h := newBoardHistory(t, uuid.New(), "草原")
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 3, 3),
		[]board.Tile{boardtest.Tile(t, 1, 1, "land"), boardtest.Tile(t, 2, 0, "base"), boardtest.Tile(t, 0, 0, "land"), boardtest.Tile(t, 1, 0, "land")},
		[]board.Connection{boardtest.Connection(1, 1, 1, 0), boardtest.Connection(0, 0, 1, 0)}))

	state := h.project()

	require.Equal(t, []readmodel.BoardTile{
		{X: 0, Y: 0, Kind: readmodel.BoardTileKindLand}, {X: 1, Y: 0, Kind: readmodel.BoardTileKindLand},
		{X: 2, Y: 0, Kind: readmodel.BoardTileKindBase}, {X: 1, Y: 1, Kind: readmodel.BoardTileKindLand},
	}, state.Metadata.Tiles)
	require.Equal(t, []readmodel.BoardConnection{
		{A: readmodel.NewBoardPosition(0, 0), B: readmodel.NewBoardPosition(1, 0)},
		{A: readmodel.NewBoardPosition(1, 0), B: readmodel.NewBoardPosition(1, 1)},
	}, state.Metadata.Connections)
	require.Equal(t, 3, state.Metadata.Width)
	require.Equal(t, int64(2), state.Metadata.Revision)
}

func TestBoard_LayoutChangeRemovesChangesAndResizes(t *testing.T) {
	h := newBoardHistory(t, uuid.New(), "草原")
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 3, 3),
		[]board.Tile{boardtest.Tile(t, 0, 0, "base"), boardtest.Tile(t, 1, 0, "land"), boardtest.Tile(t, 2, 0, "land")},
		[]board.Connection{boardtest.Connection(0, 0, 1, 0), boardtest.Connection(1, 0, 2, 0)}))
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 2, 2),
		[]board.Tile{boardtest.Tile(t, 0, 0, "base"), boardtest.Tile(t, 1, 0, "checkpoint")},
		[]board.Connection{boardtest.Connection(0, 0, 1, 0)}))

	state := h.project()

	require.Equal(t, []readmodel.BoardTile{
		{X: 0, Y: 0, Kind: readmodel.BoardTileKindBase}, {X: 1, Y: 0, Kind: readmodel.BoardTileKindCheckpoint},
	}, state.Metadata.Tiles)
	require.Equal(t, []readmodel.BoardConnection{{A: readmodel.NewBoardPosition(0, 0), B: readmodel.NewBoardPosition(1, 0)}}, state.Metadata.Connections)
	require.Equal(t, 2, state.Metadata.Width)
	require.Equal(t, 2, state.Metadata.Height)
	require.Equal(t, int64(3), state.Metadata.Revision)
}

func TestBoard_DoesNotChangeTheGivenState(t *testing.T) {
	h := newBoardHistory(t, uuid.New(), "草原")
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 3, 3), []board.Tile{boardtest.Tile(t, 0, 0, "base")}, nil))
	before := h.project()
	h.changeLayout(board.EmptyLayout(boardtest.Size(t, 3, 3)))

	h.project()
	after := before.Evolve(h.events[len(h.events)-1])

	require.Len(t, before.Metadata.Tiles, 1)
	require.Empty(t, after.Metadata.Tiles)
}

func TestBoard_StatusEventsChangeStatusAndRevision(t *testing.T) {
	h := newBoardHistory(t, uuid.New(), "草原")
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 2, 1),
		[]board.Tile{boardtest.Tile(t, 0, 0, "base"), boardtest.Tile(t, 1, 0, "checkpoint")}, []board.Connection{boardtest.Connection(0, 0, 1, 0)}))
	h.apply(&board.PublishCommand{PublishedBy: operatortest.ID(t, 10)}, boardChangedAt)
	published := h.project()
	h.apply(&board.RetireCommand{RetiredBy: operatortest.ID(t, 10)}, boardChangedAt)
	retired := h.project()

	require.Equal(t, readmodel.BoardStatusPublished, published.Status)
	require.Equal(t, readmodel.BoardStatusPublished, published.Metadata.Status)
	require.Equal(t, int64(3), published.Metadata.Revision)
	require.Equal(t, readmodel.BoardStatusRetired, retired.Status)
	require.Equal(t, readmodel.BoardStatusRetired, retired.Metadata.Status)
	require.Equal(t, int64(4), retired.Metadata.Revision)
}

func TestBoard_DeletedEventMakesTerminal(t *testing.T) {
	h := newBoardHistory(t, uuid.New(), "草原")
	h.apply(&board.DeleteCommand{DeletedBy: operatortest.ID(t, 10)}, boardChangedAt)

	require.True(t, h.project().IsTerminal())
}

func TestBoard_IgnoresEventsOfOtherAggregates(t *testing.T) {
	state := readmodel.Board{}.Evolve(registeredCreature(t, uuid.New(), "ドラゴン", boardRegisteredAt)[0])

	require.Equal(t, readmodel.Board{}, state)
}

func TestNewBoardValues_RejectUnknownValues(t *testing.T) {
	_, err := readmodel.NewBoardStatus("archived")
	require.Error(t, err)
	_, err = readmodel.NewBoardTileKind("water")
	require.Error(t, err)
}
