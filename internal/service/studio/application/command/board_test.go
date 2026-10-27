package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

func restoredBoard(t *testing.T, ctx context.Context, publicID uuid.UUID) board.Board {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := identifier.New(registry, board.NewID).Resolve(ctx, publicID)
	require.NoError(t, err)
	state, err := boardStore(registry).Load(ctx, id)
	require.NoError(t, err)
	return state.(board.Board)
}

func registerBoard(t *testing.T, ctx context.Context, commands *command.BoardHandler, name string) command.ChangedBoard {
	t.Helper()
	registered, err := commands.Register(ctx, name)
	require.NoError(t, err)
	return registered
}

func tileInput(x, y int, kind string) command.BoardTileInput {
	return command.BoardTileInput{X: x, Y: y, Kind: kind}
}

func connectionInput(ax, ay, bx, by int) command.BoardConnectionInput {
	return command.BoardConnectionInput{A: command.NewBoardPosition(ax, ay), B: command.NewBoardPosition(bx, by)}
}

// 拠点 → 土地 → チェックポイントが横に並ぶ、公開できる 3×3
func publishableLayoutInput() command.BoardLayoutInput {
	return layoutInput(3, 3,
		tiles(tileInput(0, 0, "base"), tileInput(1, 0, "land"), tileInput(2, 0, "checkpoint")),
		connections(connectionInput(0, 0, 1, 0), connectionInput(1, 0, 2, 0)))
}

// 公開できるレイアウトを入れた、準備中のボード
func publishableBoard(t *testing.T, ctx context.Context, commands *command.BoardHandler) command.ChangedBoard {
	t.Helper()
	registered := registerBoard(t, ctx, commands, "草原-"+uuid.NewString())
	changed, err := commands.ChangeLayout(ctx, registered.ID, registered.Revision, publishableLayoutInput())
	require.NoError(t, err)
	return changed
}

func TestRegisterBoard_ReturnsPublicIDAndMakesEmptyDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))

	registered, err := newBoardHandler(t, ctx).Register(ctx, "  草原 ")

	require.NoError(t, err)
	require.Equal(t, int64(1), registered.Revision)
	current := restoredBoard(t, ctx, registered.ID)
	require.Equal(t, "草原", current.Name.String())
	require.Equal(t, board.StatusDraft, current.Status)
	require.Equal(t, 8, current.Layout.Size().Width())
	require.Equal(t, 8, current.Layout.Size().Height())
	require.Empty(t, current.Layout.Tiles())
	require.Empty(t, current.Layout.Connections())
}

func TestRegisterBoard_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	registerBoard(t, ctx, commands, "ＡＢＣ")

	_, err := commands.Register(ctx, " abc")

	require.ErrorIs(t, err, command.ErrBoardNameAlreadyInUse)
}

func TestRegisterBoard_AllowsNameUsedByStarterDeck(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	_, err := newStarterDeckHandler(t, ctx).Register(ctx, "草原")
	require.NoError(t, err)

	_, err = newBoardHandler(t, ctx).Register(ctx, "草原")

	require.NoError(t, err)
}

func TestRegisterBoard_RejectsInvalidName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  input.Error
	}{
		{name: "blank", value: " ", want: &input.Required{Field: "name"}},
		{name: "too_long", value: strings.Repeat("あ", board.MaxNameLength+1), want: &input.TooLong{Field: "name", Max: board.MaxNameLength}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))

			_, err := newBoardHandler(t, ctx).Register(ctx, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
		})
	}
}

func TestBoardHandler_RejectsCallWithoutOperator(t *testing.T) {
	ctx := isolatedDatabase(t)

	_, err := newBoardHandler(t, ctx).Register(ctx, "草原")

	require.ErrorIs(t, err, command.ErrNotOperator)
}

func TestRenameBoard_ChangesNameAndFreesOldName(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	registered := registerBoard(t, ctx, commands, "草原")

	renamed, err := commands.Rename(ctx, registered.ID, registered.Revision, "砂漠")

	require.NoError(t, err)
	require.Equal(t, int64(2), renamed.Revision)
	require.Equal(t, "砂漠", restoredBoard(t, ctx, registered.ID).Name.String())
	registerBoard(t, ctx, commands, "草原")
}

func TestRenameBoard_SameNameKeepsRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	registered := registerBoard(t, ctx, commands, "草原")

	renamed, err := commands.Rename(ctx, registered.ID, registered.Revision, "草原")

	require.NoError(t, err)
	require.Equal(t, registered.Revision, renamed.Revision)
}

func TestRenameBoard_RejectsNameUsedByOtherBoard(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	registerBoard(t, ctx, commands, "草原")
	other := registerBoard(t, ctx, commands, "砂漠")

	_, err := commands.Rename(ctx, other.ID, other.Revision, "草原")

	require.ErrorIs(t, err, command.ErrBoardNameAlreadyInUse)
}

func TestBoardHandler_RejectsStaleRevisionAndMissingBoard(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	registered := registerBoard(t, ctx, commands, "草原")
	_, err := commands.Rename(ctx, registered.ID, registered.Revision, "砂漠")
	require.NoError(t, err)
	operations := []struct {
		name string
		call func(id uuid.UUID, revision int64) error
	}{
		{name: "rename", call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Rename(ctx, id, revision, "湖")
			return err
		}},
		{name: "change_layout", call: func(id uuid.UUID, revision int64) error {
			_, err := commands.ChangeLayout(ctx, id, revision, publishableLayoutInput())
			return err
		}},
		{name: "publish", call: func(id uuid.UUID, revision int64) error { _, err := commands.Publish(ctx, id, revision); return err }},
		{name: "retire", call: func(id uuid.UUID, revision int64) error { _, err := commands.Retire(ctx, id, revision); return err }},
		{name: "delete", call: func(id uuid.UUID, revision int64) error { return commands.Delete(ctx, id, revision) }},
	}
	for _, op := range operations {
		t.Run(op.name+"_stale_revision", func(t *testing.T) {
			require.ErrorIs(t, op.call(registered.ID, registered.Revision), command.ErrRevisionConflict)
		})
		t.Run(op.name+"_missing_board", func(t *testing.T) {
			require.ErrorIs(t, op.call(uuid.New(), 1), command.ErrBoardNotFound)
		})
	}
}

func TestChangeBoardLayout_SavesWholeLayoutAndReplacesPrevious(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	changed := publishableBoard(t, ctx, commands)

	current := restoredBoard(t, ctx, changed.ID)
	require.Equal(t, int64(2), changed.Revision)
	require.Equal(t, 3, current.Layout.Size().Width())
	require.Len(t, current.Layout.Tiles(), 3)
	require.Len(t, current.Layout.Connections(), 2)

	replaced, err := commands.ChangeLayout(ctx, changed.ID, changed.Revision, layoutInput(2, 1, tiles(tileInput(1, 0, "base")), nil))
	require.NoError(t, err)

	current = restoredBoard(t, ctx, changed.ID)
	require.Equal(t, int64(3), replaced.Revision)
	require.Equal(t, 2, current.Layout.Size().Width())
	require.Len(t, current.Layout.Tiles(), 1)
	require.Empty(t, current.Layout.Connections())
}

func TestChangeBoardLayout_SameLayoutKeepsRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	changed := publishableBoard(t, ctx, commands)

	again, err := commands.ChangeLayout(ctx, changed.ID, changed.Revision, publishableLayoutInput())

	require.NoError(t, err)
	require.Equal(t, changed.Revision, again.Revision)
}

// 幅・高さ・マス・つながりから、変えた後のレイアウトの入力を作る
func layoutInput(width, height int, tiles []command.BoardTileInput, connections []command.BoardConnectionInput) command.BoardLayoutInput {
	return command.BoardLayoutInput{Width: width, Height: height, Tiles: tiles, Connections: connections}
}

func tiles(inputs ...command.BoardTileInput) []command.BoardTileInput {
	return inputs
}

func connections(inputs ...command.BoardConnectionInput) []command.BoardConnectionInput {
	return inputs
}

// ValidateLayout の違反を、入力の項目のパスに写す。つながりの a・b は、入力で書いた側のパスにする
func TestChangeBoardLayout_MapsViolationsToInputPaths(t *testing.T) {
	tests := []struct {
		name   string
		layout command.BoardLayoutInput
		want   []input.Error
	}{
		{
			name:   "width_out_of_range_skips_tiles",
			layout: layoutInput(33, 1, tiles(tileInput(99, 99, "land")), nil),
			want:   []input.Error{&input.OutOfRange{Field: "width", Min: 1, Max: 32}},
		},
		{
			name:   "width_and_height_out_of_range",
			layout: layoutInput(0, -1, nil, nil),
			want: []input.Error{
				&input.OutOfRange{Field: "width", Min: 1, Max: 32},
				&input.OutOfRange{Field: "height", Min: 1, Max: 32},
			},
		},
		{
			name:   "tile_out_of_range",
			layout: layoutInput(3, 2, tiles(tileInput(0, 0, "land"), tileInput(3, 2, "land")), nil),
			want: []input.Error{
				&input.OutOfRange{Field: "tiles.1.x", Min: 0, Max: 2},
				&input.OutOfRange{Field: "tiles.1.y", Min: 0, Max: 1},
			},
		},
		{
			name:   "duplicated_tile",
			layout: layoutInput(3, 3, tiles(tileInput(1, 1, "land"), tileInput(1, 1, "base")), nil),
			want:   []input.Error{&input.Duplicated{Field: "tiles.1"}},
		},
		{
			name:   "unknown_tile_kind",
			layout: layoutInput(3, 3, tiles(tileInput(1, 1, "water")), nil),
			want:   []input.Error{&input.InvalidFormat{Field: "tiles.0.kind"}},
		},
		{
			name:   "connection_end_a_without_tile",
			layout: layoutInput(3, 3, tiles(tileInput(1, 0, "land")), connections(connectionInput(0, 0, 1, 0))),
			want:   []input.Error{&input.NotFound{Field: "connections.0.a"}},
		},
		{
			name:   "connection_end_b_without_tile_when_ends_are_swapped_in_input",
			layout: layoutInput(3, 3, tiles(tileInput(1, 0, "land")), connections(connectionInput(1, 0, 0, 0))),
			want:   []input.Error{&input.NotFound{Field: "connections.0.b"}},
		},
		{
			name:   "not_adjacent",
			layout: layoutInput(3, 3, tiles(tileInput(0, 0, "land"), tileInput(1, 1, "land")), connections(connectionInput(0, 0, 1, 1))),
			want:   []input.Error{&input.NotAdjacent{Field: "connections.0"}},
		},
		{
			name:   "duplicated_connection",
			layout: layoutInput(3, 3, tiles(tileInput(0, 0, "land"), tileInput(1, 0, "land")), connections(connectionInput(0, 0, 1, 0), connectionInput(1, 0, 0, 0))),
			want:   []input.Error{&input.Duplicated{Field: "connections.1"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			commands := newBoardHandler(t, ctx)
			registered := registerBoard(t, ctx, commands, "草原")

			_, err := commands.ChangeLayout(ctx, registered.ID, registered.Revision, tt.layout)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.Equal(t, int64(1), restoredBoard(t, ctx, registered.ID).Version)
		})
	}
}

func TestPublishBoard_MakesPublishedAndSecondCallKeepsRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	changed := publishableBoard(t, ctx, commands)

	published, err := commands.Publish(ctx, changed.ID, changed.Revision)
	require.NoError(t, err)
	again, err := commands.Publish(ctx, published.ID, published.Revision)

	require.NoError(t, err)
	require.Equal(t, changed.Revision+1, published.Revision)
	require.Equal(t, published.Revision, again.Revision)
	require.Equal(t, board.StatusPublished, restoredBoard(t, ctx, changed.ID).Status)
}

func TestPublishBoard_ReportsCountsAndUnreachableTiles(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	registered := registerBoard(t, ctx, commands, "草原")
	changed, err := commands.ChangeLayout(ctx, registered.ID, registered.Revision, command.BoardLayoutInput{
		Width: 3, Height: 3,
		Tiles:       []command.BoardTileInput{tileInput(0, 0, "base"), tileInput(1, 0, "checkpoint"), tileInput(2, 2, "land")},
		Connections: []command.BoardConnectionInput{connectionInput(0, 0, 1, 0)},
	})
	require.NoError(t, err)

	_, err = commands.Publish(ctx, changed.ID, changed.Revision)

	require.ErrorIs(t, err, command.ErrBoardNotPublishable)
	var notPublishable *command.BoardNotPublishableError
	require.ErrorAs(t, err, &notPublishable)
	require.Equal(t, &command.BoardNotPublishableError{
		BaseCount: 1, CheckpointCount: 1, UnreachableTiles: []command.BoardPosition{command.NewBoardPosition(2, 2)},
	}, notPublishable)
	require.Equal(t, board.StatusDraft, restoredBoard(t, ctx, changed.ID).Status)
}

func TestRetireBoard_MakesRetiredOnlyFromPublishedAndCanBePublishedAgain(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	changed := publishableBoard(t, ctx, commands)

	_, err := commands.Retire(ctx, changed.ID, changed.Revision)
	require.ErrorIs(t, err, command.ErrInvalidBoardStatusTransition)

	published, err := commands.Publish(ctx, changed.ID, changed.Revision)
	require.NoError(t, err)
	retired, err := commands.Retire(ctx, published.ID, published.Revision)
	require.NoError(t, err)
	require.Equal(t, board.StatusRetired, restoredBoard(t, ctx, changed.ID).Status)
	again, err := commands.Publish(ctx, retired.ID, retired.Revision)
	require.NoError(t, err)
	require.Equal(t, board.StatusPublished, restoredBoard(t, ctx, again.ID).Status)
}

func TestBoardHandler_RejectsEditAndDeleteUnlessDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	changed := publishableBoard(t, ctx, commands)
	published, err := commands.Publish(ctx, changed.ID, changed.Revision)
	require.NoError(t, err)

	_, err = commands.Rename(ctx, published.ID, published.Revision, "砂漠")
	require.ErrorIs(t, err, command.ErrBoardNotDraft)
	// 入力の誤りより先に、準備中でないことを返す
	_, err = commands.ChangeLayout(ctx, published.ID, published.Revision, publishableLayoutInput())
	require.ErrorIs(t, err, command.ErrBoardNotDraft)
	require.ErrorIs(t, commands.Delete(ctx, published.ID, published.Revision), command.ErrBoardNotDraft)
}

func TestDeleteBoard_RemovesBoardAndFreesName(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	registered := registerBoard(t, ctx, commands, "草原")

	require.NoError(t, commands.Delete(ctx, registered.ID, registered.Revision))

	_, err := commands.Rename(ctx, registered.ID, registered.Revision, "砂漠")
	require.ErrorIs(t, err, command.ErrBoardNotFound)
	registerBoard(t, ctx, commands, "草原")
}
