package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules/studiorulestest"
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

// 土地には火属性を付ける（属性を確かめないテストで、属性の誤りが出ないように）
func tileInput(x, y int, kind string) command.BoardTileInput {
	if kind == "land" {
		return tileInputWithAttribute(x, y, kind, "fire")
	}
	return command.BoardTileInput{X: x, Y: y, Kind: kind}
}

func tileInputWithAttribute(x, y int, kind, attribute string) command.BoardTileInput {
	return command.BoardTileInput{X: x, Y: y, Kind: kind, Attribute: attribute}
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

// 8×8 を、studio_rules の範囲の中に収める
func TestRegisterBoard_ClampsDefaultSizeIntoStudioRulesRange(t *testing.T) {
	tests := []struct {
		name  string
		board studiorules.Board
		want  int
	}{
		{name: "min_above_default", board: studiorulestest.Board(t, 10, 20), want: 10},
		{name: "max_below_default", board: studiorulestest.Board(t, 1, 5), want: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			commands := newBoardHandlerWithRules(t, ctx, studiorules.Rules{Board: tt.board})

			registered := registerBoard(t, ctx, commands, "草原")

			current := restoredBoard(t, ctx, registered.ID)
			require.Equal(t, tt.want, current.Layout.Size().Width())
			require.Equal(t, tt.want, current.Layout.Size().Height())
		})
	}
}

func TestRegisterBoard_StartsWithInitialDefaultRules(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))

	registered := registerBoard(t, ctx, newBoardHandler(t, ctx), "草原")

	require.Equal(t, board.InitialDefaultRules(), restoredBoard(t, ctx, registered.ID).DefaultRules)
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
	commands := newBoardHandler(t, ctx)

	_, err := commands.Register(ctx, "草原")
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.ChangeDefaultRules(ctx, uuid.New(), 1, command.BoardDefaultRulesInput{TargetMagic: 5000})
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.CheckLayout(ctx, publishableLayoutInput())
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
		{name: "change_default_rules", call: func(id uuid.UUID, revision int64) error {
			_, err := commands.ChangeDefaultRules(ctx, id, revision, command.NewBoardDefaultRulesInput(5000, 0, 200, 50))
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

func pairInput(ax, ay, bx, by int) command.BoardPairInput {
	return command.BoardPairInput{A: command.NewBoardPosition(ax, ay), B: command.NewBoardPosition(bx, by)}
}

func withPairs(layout command.BoardLayoutInput, pairs ...command.BoardPairInput) command.BoardLayoutInput {
	layout.Pairs = pairs
	return layout
}

func tiles(inputs ...command.BoardTileInput) []command.BoardTileInput {
	return inputs
}

func connections(inputs ...command.BoardConnectionInput) []command.BoardConnectionInput {
	return inputs
}

// 範囲は固定の値ではなく studio_rules から読む。範囲を変えた後は、準備中のボードを保存し直すときも新しい範囲で確かめる
func TestChangeBoardLayout_UsesStudioRulesRange(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerBoard(t, ctx, newBoardHandler(t, ctx), "草原")
	narrowed := newBoardHandlerWithRules(t, ctx, studiorulestest.Rules(t, 2, 4))

	_, err := narrowed.ChangeLayout(ctx, registered.ID, registered.Revision, layoutInput(8, 8, nil, nil))
	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, input.Errors{
		&input.OutOfRange{Field: "width", Min: 2, Max: 4},
		&input.OutOfRange{Field: "height", Min: 2, Max: 4},
	}, invalid)
	_, err = narrowed.ChangeLayout(ctx, registered.ID, registered.Revision, layoutInput(1, 4, nil, nil))
	invalid, ok = input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, input.Errors{&input.OutOfRange{Field: "width", Min: 2, Max: 4}}, invalid)

	changed, err := narrowed.ChangeLayout(ctx, registered.ID, registered.Revision, layoutInput(4, 2, nil, nil))
	require.NoError(t, err)
	require.Equal(t, int64(2), changed.Revision)
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
				&input.OutOfRange{Field: "tiles.1.land.x", Min: 0, Max: 2},
				&input.OutOfRange{Field: "tiles.1.land.y", Min: 0, Max: 1},
			},
		},
		{
			name:   "base_out_of_range",
			layout: layoutInput(3, 2, tiles(tileInput(3, 0, "base")), nil),
			want:   []input.Error{&input.OutOfRange{Field: "tiles.0.base.x", Min: 0, Max: 2}},
		},
		{
			name:   "checkpoint_out_of_range",
			layout: layoutInput(3, 2, tiles(tileInput(0, 2, "checkpoint")), nil),
			want:   []input.Error{&input.OutOfRange{Field: "tiles.0.checkpoint.y", Min: 0, Max: 1}},
		},
		{
			name:   "duplicated_tile",
			layout: layoutInput(3, 3, tiles(tileInput(1, 1, "land"), tileInput(1, 1, "base")), nil),
			want:   []input.Error{&input.Duplicated{Field: "tiles.1"}},
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
			name:   "prophecy_shop_out_of_range_uses_input_field_name",
			layout: layoutInput(3, 2, tiles(tileInput(3, 0, "prophecy_shop")), nil),
			want:   []input.Error{&input.OutOfRange{Field: "tiles.0.prophecyShop.x", Min: 0, Max: 2}},
		},
		{
			name:   "card_shop_out_of_range_uses_input_field_name",
			layout: layoutInput(3, 2, tiles(tileInput(0, 2, "card_shop")), nil),
			want:   []input.Error{&input.OutOfRange{Field: "tiles.0.cardShop.y", Min: 0, Max: 1}},
		},
		{
			name:   "pair_end_a_without_tile",
			layout: withPairs(layoutInput(3, 3, tiles(tileInput(1, 0, "warp")), nil), pairInput(0, 0, 1, 0)),
			want:   []input.Error{&input.NotFound{Field: "pairs.0.a"}},
		},
		{
			name:   "pair_end_b_without_tile_when_ends_are_swapped_in_input",
			layout: withPairs(layoutInput(3, 3, tiles(tileInput(1, 0, "bridge")), nil), pairInput(1, 0, 0, 0)),
			want:   []input.Error{&input.NotFound{Field: "pairs.0.b"}},
		},
		{
			name:   "pair_end_is_not_pairable_when_ends_are_swapped_in_input",
			layout: withPairs(layoutInput(3, 3, tiles(tileInput(0, 0, "warp"), tileInput(1, 0, "land")), nil), pairInput(1, 0, 0, 0)),
			want:   []input.Error{&input.NotPairable{Field: "pairs.0.a"}},
		},
		{
			name:   "pair_of_different_kinds",
			layout: withPairs(layoutInput(3, 3, tiles(tileInput(0, 0, "warp"), tileInput(2, 2, "bridge")), nil), pairInput(0, 0, 2, 2)),
			want:   []input.Error{&input.PairKindMismatch{Field: "pairs.0"}},
		},
		{
			name:   "pair_with_itself",
			layout: withPairs(layoutInput(3, 3, tiles(tileInput(0, 0, "warp")), nil), pairInput(0, 0, 0, 0)),
			want:   []input.Error{&input.PairedWithSelf{Field: "pairs.0"}},
		},
		{
			name:   "tile_in_two_pairs",
			layout: withPairs(layoutInput(3, 3, tiles(tileInput(0, 0, "warp"), tileInput(1, 0, "warp"), tileInput(2, 0, "warp")), nil), pairInput(0, 0, 1, 0), pairInput(2, 0, 0, 0)),
			want:   []input.Error{&input.Duplicated{Field: "pairs.1.b"}},
		},
		{
			name:   "duplicated_pair",
			layout: withPairs(layoutInput(3, 3, tiles(tileInput(0, 0, "warp"), tileInput(2, 2, "warp")), nil), pairInput(0, 0, 2, 2), pairInput(2, 2, 0, 0)),
			want:   []input.Error{&input.Duplicated{Field: "pairs.1"}},
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

func TestChangeBoardLayout_ReturnsInternalErrorForShapesTheInputRulesOut(t *testing.T) {
	tests := []struct {
		name string
		tile command.BoardTileInput
	}{
		{name: "unknown_kind", tile: tileInput(1, 1, "water")},
		{name: "unknown_attribute", tile: tileInputWithAttribute(1, 1, "land", "light")},
		{name: "land_without_attribute", tile: tileInputWithAttribute(1, 1, "land", "")},
		{name: "base_with_attribute", tile: tileInputWithAttribute(1, 1, "base", "fire")},
		{name: "checkpoint_with_attribute", tile: tileInputWithAttribute(1, 1, "checkpoint", "multi")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			commands := newBoardHandler(t, ctx)
			registered := registerBoard(t, ctx, commands, "草原")

			_, err := commands.ChangeLayout(ctx, registered.ID, registered.Revision, layoutInput(3, 3, tiles(tt.tile), nil))

			require.Error(t, err)
			_, isInput := input.AsErrors(err)
			require.False(t, isInput)
		})
	}
}

// 離れた転送円・橋の対と、施設・相手のいない転送円も保存でき、読み直すと同じ形で戻る
func TestChangeBoardLayout_SavesSpecialTilesAndPairs(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	registered := registerBoard(t, ctx, commands, "草原")
	layout := withPairs(layoutInput(4, 2,
		tiles(
			tileInput(0, 0, "warp"), tileInput(3, 1, "warp"), tileInput(1, 0, "bridge"), tileInput(2, 1, "bridge"), tileInput(2, 0, "warp"),
			tileInput(0, 1, "sanctuary"), tileInput(1, 1, "shrine"), tileInput(3, 0, "prophecy_shop"),
		), nil), pairInput(3, 1, 0, 0), pairInput(1, 0, 2, 1))

	changed, err := commands.ChangeLayout(ctx, registered.ID, registered.Revision, layout)

	require.NoError(t, err)
	current := restoredBoard(t, ctx, changed.ID)
	require.Len(t, current.Layout.Tiles(), 8)
	require.Equal(t, []board.Pair{boardtest.Pair(0, 0, 3, 1), boardtest.Pair(1, 0, 2, 1)}, current.Layout.Pairs())
}

func TestChangeBoardDefaultRules_SavesAllValuesAndKeepsLayout(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	changed := publishableBoard(t, ctx, commands)

	rulesChanged, err := commands.ChangeDefaultRules(ctx, changed.ID, changed.Revision,
		command.NewBoardDefaultRulesInput(5000, 0, 200, 50))

	require.NoError(t, err)
	require.Equal(t, changed.Revision+1, rulesChanged.Revision)
	current := restoredBoard(t, ctx, changed.ID)
	require.Equal(t, boardtest.DefaultRules(t, 5000, 0, 200, 50), current.DefaultRules)
	require.Len(t, current.Layout.Tiles(), 3)
}

func TestChangeBoardDefaultRules_SameValuesKeepRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	registered := registerBoard(t, ctx, commands, "草原")

	same, err := commands.ChangeDefaultRules(ctx, registered.ID, registered.Revision,
		command.NewBoardDefaultRulesInput(8000, 600, 300, 100))

	require.NoError(t, err)
	require.Equal(t, registered.Revision, same.Revision)
}

func TestChangeBoardDefaultRules_MapsViolationsToInputPaths(t *testing.T) {
	tests := []struct {
		name  string
		rules command.BoardDefaultRulesInput
		want  []input.Error
	}{
		{
			name:  "zero_target",
			rules: command.NewBoardDefaultRulesInput(0, 0, 0, 0),
			want: []input.Error{
				&input.TooSmall{Field: "targetMagic", Min: 1},
				&input.NotGreaterThan{Field: "targetMagic", Than: "startingMagic"},
			},
		},
		{
			name:  "negative_starting",
			rules: command.NewBoardDefaultRulesInput(100, -1, 0, 0),
			want:  []input.Error{&input.TooSmall{Field: "startingMagic", Min: 0}},
		},
		{
			name:  "negative_bonuses",
			rules: command.NewBoardDefaultRulesInput(100, 0, -1, -2),
			want: []input.Error{
				&input.TooSmall{Field: "lapBonus", Min: 0},
				&input.TooSmall{Field: "checkpointBonus", Min: 0},
			},
		},
		{
			name:  "target_equal_to_starting",
			rules: command.NewBoardDefaultRulesInput(600, 600, 0, 0),
			want:  []input.Error{&input.NotGreaterThan{Field: "targetMagic", Than: "startingMagic"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			commands := newBoardHandler(t, ctx)
			registered := registerBoard(t, ctx, commands, "草原")

			_, err := commands.ChangeDefaultRules(ctx, registered.ID, registered.Revision, tt.rules)

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

func TestPublishBoard_ReportsProblemsWithPositions(t *testing.T) {
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
		Problems: []command.BoardPublishProblem{
			{Kind: command.BoardPublishProblemUnreachableTiles, Positions: []command.BoardPosition{command.NewBoardPosition(2, 2)}},
		},
	}, notPublishable)
	require.Equal(t, board.StatusDraft, restoredBoard(t, ctx, changed.ID).Status)
}

func TestPublishBoard_ReportsUnpairedWarpsAndBridges(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	registered := registerBoard(t, ctx, commands, "草原")
	changed, err := commands.ChangeLayout(ctx, registered.ID, registered.Revision, layoutInput(3, 3,
		tiles(tileInput(0, 0, "base"), tileInput(1, 0, "checkpoint"), tileInput(2, 0, "warp"), tileInput(0, 1, "bridge")),
		connections(connectionInput(0, 0, 1, 0), connectionInput(1, 0, 2, 0), connectionInput(0, 0, 0, 1))))
	require.NoError(t, err)

	_, err = commands.Publish(ctx, changed.ID, changed.Revision)

	var notPublishable *command.BoardNotPublishableError
	require.ErrorAs(t, err, &notPublishable)
	require.Equal(t, []command.BoardPublishProblem{
		{Kind: command.BoardPublishProblemUnpairedTiles, Positions: []command.BoardPosition{command.NewBoardPosition(2, 0), command.NewBoardPosition(0, 1)}},
	}, notPublishable.Problems)
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
	_, err = commands.ChangeDefaultRules(ctx, published.ID, published.Revision,
		command.NewBoardDefaultRulesInput(5000, 0, 200, 50))
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

func TestCheckBoardLayout_ReportsProblemsLikePublishAndSavesNothing(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandler(t, ctx)
	registered := registerBoard(t, ctx, commands, "草原")
	unreachable := command.BoardLayoutInput{
		Width: 3, Height: 3,
		Tiles:       []command.BoardTileInput{tileInput(0, 0, "base"), tileInput(1, 0, "checkpoint"), tileInput(2, 2, "land")},
		Connections: []command.BoardConnectionInput{connectionInput(0, 0, 1, 0)},
	}

	empty, err := commands.CheckLayout(ctx, layoutInput(3, 3, nil, nil))
	require.NoError(t, err)
	notPublishable, err := commands.CheckLayout(ctx, unreachable)
	require.NoError(t, err)
	publishable, err := commands.CheckLayout(ctx, publishableLayoutInput())
	require.NoError(t, err)

	require.Equal(t, []command.BoardPublishProblem{
		{Kind: command.BoardPublishProblemBaseCount, Positions: []command.BoardPosition{}},
		{Kind: command.BoardPublishProblemNoCheckpoint, Positions: []command.BoardPosition{}},
	}, empty)
	require.Equal(t, []command.BoardPublishProblem{
		{Kind: command.BoardPublishProblemUnreachableTiles, Positions: []command.BoardPosition{command.NewBoardPosition(2, 2)}},
	}, notPublishable)
	require.Empty(t, publishable)
	// 保存していないので、ボードは登録の直後のまま（版が進まず、同じ revision で保存できる）
	current := restoredBoard(t, ctx, registered.ID)
	require.Equal(t, registered.Revision, current.Version)
	require.Empty(t, current.Layout.Tiles())
}

// 入力の誤りは保存（ChangeLayout）と同じ写し方を通る。範囲は保存と同じく studio_rules から読む
func TestCheckBoardLayout_ReturnsSameInputErrorsAsChangeLayout(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newBoardHandlerWithRules(t, ctx, studiorulestest.Rules(t, 2, 4))
	registered := registerBoard(t, ctx, commands, "草原")
	layouts := []command.BoardLayoutInput{
		layoutInput(8, 1, tiles(tileInput(99, 99, "land")), nil),
		layoutInput(3, 3, tiles(tileInput(5, 0, "base"), tileInput(0, 0, "land"), tileInput(0, 0, "base")), connections(connectionInput(0, 0, 1, 1), connectionInput(2, 2, 1, 2))),
	}

	for _, layout := range layouts {
		_, saveErr := commands.ChangeLayout(ctx, registered.ID, registered.Revision, layout)
		_, checkErr := commands.CheckLayout(ctx, layout)

		invalid, ok := input.AsErrors(checkErr)
		require.True(t, ok)
		require.NotEmpty(t, invalid)
		require.Equal(t, saveErr, checkErr)
	}
}
