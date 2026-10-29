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
		Tiles: []readmodel.BoardTile{}, Connections: []readmodel.BoardConnection{}, Pairs: []readmodel.BoardPair{},
		DefaultRules: readmodel.NewBoardDefaultRules(8000, 600, 300, 100), TileCounts: readmodel.NewBoardTileCounts(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0),
		PublishProblems: []readmodel.BoardPublishProblem{
			readmodel.NewBoardPublishProblem(readmodel.BoardPublishProblemBaseCount),
			readmodel.NewBoardPublishProblem(readmodel.BoardPublishProblemNoCheckpoint),
		},
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

func TestBoard_DefaultRulesChangeReplacesAllValues(t *testing.T) {
	h := newBoardHistory(t, uuid.New(), "草原")
	h.apply(&board.ChangeDefaultRulesCommand{DefaultRules: boardtest.DefaultRules(t, 5000, 0, 200, 50), ChangedBy: operatortest.ID(t, 10)}, boardChangedAt)

	state := h.project()

	require.Equal(t, readmodel.NewBoardDefaultRules(5000, 0, 200, 50), state.Metadata.DefaultRules)
	require.Equal(t, int64(2), state.Metadata.Revision)
	require.Equal(t, boardChangedAt, state.Metadata.UpdatedAt)
}

// マスは y → x、つながりは a → b の順に並べる。差分を当てた結果が、入れた順によらず同じ並びになる
func TestBoard_LayoutChangeAppliesDifferenceInOrder(t *testing.T) {
	h := newBoardHistory(t, uuid.New(), "草原")
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 3, 3),
		[]board.Tile{boardtest.Tile(t, 1, 1, "land", "fire"), boardtest.Tile(t, 2, 0, "base", ""), boardtest.Tile(t, 0, 0, "land", "fire"), boardtest.Tile(t, 1, 0, "land", "fire")},
		[]board.Connection{boardtest.Connection(1, 1, 1, 0), boardtest.Connection(0, 0, 1, 0)}))

	state := h.project()

	require.Equal(t, []readmodel.BoardTile{
		{X: 0, Y: 0, Kind: readmodel.BoardTileKindLand, Attribute: readmodel.BoardTileAttributeFire},
		{X: 1, Y: 0, Kind: readmodel.BoardTileKindLand, Attribute: readmodel.BoardTileAttributeFire},
		{X: 2, Y: 0, Kind: readmodel.BoardTileKindBase},
		{X: 1, Y: 1, Kind: readmodel.BoardTileKindLand, Attribute: readmodel.BoardTileAttributeFire},
	}, state.Metadata.Tiles)
	require.Equal(t, []readmodel.BoardConnection{
		{A: readmodel.NewBoardPosition(0, 0), B: readmodel.NewBoardPosition(1, 0)},
		{A: readmodel.NewBoardPosition(1, 0), B: readmodel.NewBoardPosition(1, 1)},
	}, state.Metadata.Connections)
	require.Equal(t, 3, state.Metadata.Width)
	require.Equal(t, int64(2), state.Metadata.Revision)
}

// 差分を当てるたびに、置いてあるマス全体から数え直す（置く・消す・種類か属性を変えるのどれでも合う）
func TestBoard_LayoutChangeRecountsTilesByKindAndAttribute(t *testing.T) {
	h := newBoardHistory(t, uuid.New(), "草原")
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 4, 2),
		[]board.Tile{
			boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "land", "fire"), boardtest.Tile(t, 2, 0, "land", "fire"),
			boardtest.Tile(t, 3, 0, "checkpoint", ""), boardtest.Tile(t, 0, 1, "land", "earth"), boardtest.Tile(t, 1, 1, "land", "multi"),
		}, nil))
	placed := h.project().Metadata.TileCounts
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 4, 2),
		[]board.Tile{
			boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "land", "water"), boardtest.Tile(t, 3, 0, "checkpoint", ""),
			boardtest.Tile(t, 0, 1, "checkpoint", ""), boardtest.Tile(t, 1, 1, "land", "neutral"), boardtest.Tile(t, 2, 1, "land", "wind"),
		}, nil))

	changed := h.project().Metadata.TileCounts

	require.Equal(t, readmodel.NewBoardTileCounts(4, 1, 1, 2, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0), placed)
	require.Equal(t, readmodel.NewBoardTileCounts(3, 1, 2, 0, 0, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0), changed)
}

// 差分を当てるたびに、置いてあるマス全体から公開できない理由を計算し直す（投影で持ち、読むときに計算しない）
func TestBoard_LayoutChangeRecomputesPublishProblems(t *testing.T) {
	h := newBoardHistory(t, uuid.New(), "草原")
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 3, 3),
		[]board.Tile{boardtest.Tile(t, 1, 1, "base", ""), boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 2, 2, "land", "fire")}, nil))
	twoBases := h.project().Metadata.PublishProblems
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 3, 3),
		[]board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", ""), boardtest.Tile(t, 2, 2, "land", "fire")},
		[]board.Connection{boardtest.Connection(0, 0, 1, 0)}))
	unreachable := h.project().Metadata.PublishProblems
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 3, 3),
		[]board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", "")},
		[]board.Connection{boardtest.Connection(0, 0, 1, 0)}))
	publishable := h.project().Metadata.PublishProblems

	require.Equal(t, []readmodel.BoardPublishProblem{
		readmodel.NewBoardPublishProblem(readmodel.BoardPublishProblemBaseCount, readmodel.NewBoardPosition(0, 0), readmodel.NewBoardPosition(1, 1)),
		readmodel.NewBoardPublishProblem(readmodel.BoardPublishProblemNoCheckpoint),
	}, twoBases)
	require.Equal(t, []readmodel.BoardPublishProblem{
		readmodel.NewBoardPublishProblem(readmodel.BoardPublishProblemUnreachableTiles, readmodel.NewBoardPosition(2, 2)),
	}, unreachable)
	require.Equal(t, []readmodel.BoardPublishProblem{}, publishable)
}

// 対の差分を当て、切れた対（マスを消した・種類を変えた）は外れる。公開できない理由と数も、置いてある全体から計算し直す
func TestBoard_LayoutChangeAppliesPairsAndRecomputesUnpairedProblem(t *testing.T) {
	size := boardtest.Size(t, 4, 2)
	tiles := []board.Tile{
		boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", ""),
		boardtest.Tile(t, 2, 0, "warp", ""), boardtest.Tile(t, 3, 0, "warp", ""), boardtest.Tile(t, 0, 1, "bridge", ""), boardtest.Tile(t, 1, 1, "bridge", ""),
		boardtest.Tile(t, 2, 1, "sanctuary", ""), boardtest.Tile(t, 3, 1, "card_shop", ""),
	}
	connections := []board.Connection{
		boardtest.Connection(0, 0, 1, 0), boardtest.Connection(0, 0, 0, 1), boardtest.Connection(0, 1, 1, 1),
		boardtest.Connection(1, 0, 2, 0), boardtest.Connection(2, 0, 2, 1), boardtest.Connection(3, 0, 3, 1),
	}
	h := newBoardHistory(t, uuid.New(), "草原")
	h.changeLayout(boardtest.Layout(t, size, tiles, connections, boardtest.Pair(3, 0, 2, 0), boardtest.Pair(0, 1, 1, 1)))
	paired := h.project().Metadata
	h.changeLayout(boardtest.Layout(t, size, tiles, connections, boardtest.Pair(2, 0, 3, 0)))
	unpaired := h.project().Metadata

	require.Equal(t, []readmodel.BoardPair{
		{A: readmodel.NewBoardPosition(2, 0), B: readmodel.NewBoardPosition(3, 0)},
		{A: readmodel.NewBoardPosition(0, 1), B: readmodel.NewBoardPosition(1, 1)},
	}, paired.Pairs)
	require.Equal(t, []readmodel.BoardPublishProblem{}, paired.PublishProblems)
	require.Equal(t, readmodel.NewBoardTileCounts(0, 1, 1, 0, 0, 0, 0, 0, 0, 2, 2, 1, 0, 0, 1), paired.TileCounts)
	require.Equal(t, []readmodel.BoardPair{{A: readmodel.NewBoardPosition(2, 0), B: readmodel.NewBoardPosition(3, 0)}}, unpaired.Pairs)
	require.Equal(t, []readmodel.BoardPublishProblem{
		readmodel.NewBoardPublishProblem(readmodel.BoardPublishProblemUnpairedTiles, readmodel.NewBoardPosition(0, 1), readmodel.NewBoardPosition(1, 1)),
	}, unpaired.PublishProblems)
}

// マスを消して切れた対は、イベントの Unpaired に入っているので read model からも外れる
func TestBoard_LayoutChangeDropsPairOfRemovedTile(t *testing.T) {
	size := boardtest.Size(t, 3, 1)
	h := newBoardHistory(t, uuid.New(), "草原")
	h.changeLayout(boardtest.Layout(t, size, []board.Tile{boardtest.Tile(t, 0, 0, "warp", ""), boardtest.Tile(t, 2, 0, "warp", "")}, nil, boardtest.Pair(0, 0, 2, 0)))
	h.changeLayout(boardtest.Layout(t, size, []board.Tile{boardtest.Tile(t, 0, 0, "warp", "")}, nil))

	state := h.project()

	require.Equal(t, []readmodel.BoardPair{}, state.Metadata.Pairs)
}

func TestBoard_LayoutChangeReplacesAttributeOfChangedTile(t *testing.T) {
	h := newBoardHistory(t, uuid.New(), "草原")
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 2, 1),
		[]board.Tile{boardtest.Tile(t, 0, 0, "land", "fire"), boardtest.Tile(t, 1, 0, "land", "water")}, nil))
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 2, 1),
		[]board.Tile{boardtest.Tile(t, 0, 0, "land", "fire"), boardtest.Tile(t, 1, 0, "checkpoint", "")}, nil))
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 2, 1),
		[]board.Tile{boardtest.Tile(t, 0, 0, "land", "multi"), boardtest.Tile(t, 1, 0, "checkpoint", "")}, nil))

	state := h.project()

	require.Equal(t, []readmodel.BoardTile{
		{X: 0, Y: 0, Kind: readmodel.BoardTileKindLand, Attribute: readmodel.BoardTileAttributeMulti},
		{X: 1, Y: 0, Kind: readmodel.BoardTileKindCheckpoint},
	}, state.Metadata.Tiles)
}

func TestBoard_LayoutChangeRemovesChangesAndResizes(t *testing.T) {
	h := newBoardHistory(t, uuid.New(), "草原")
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 3, 3),
		[]board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "land", "fire"), boardtest.Tile(t, 2, 0, "land", "fire")},
		[]board.Connection{boardtest.Connection(0, 0, 1, 0), boardtest.Connection(1, 0, 2, 0)}))
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 2, 2),
		[]board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", "")},
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
	h.changeLayout(boardtest.Layout(t, boardtest.Size(t, 3, 3), []board.Tile{boardtest.Tile(t, 0, 0, "base", "")}, nil))
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
		[]board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", "")}, []board.Connection{boardtest.Connection(0, 0, 1, 0)}))
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
