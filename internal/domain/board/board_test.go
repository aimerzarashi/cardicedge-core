package board_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules/studiorulestest"
)

var (
	registeredAt = time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	changedAt    = time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
)

type fixture struct {
	id         board.ID
	name       board.Name
	operatorID operator.ID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{
		id:         boardtest.ID(t, 30),
		name:       boardtest.Name(t, "草原"),
		operatorID: operatortest.ID(t, 10),
	}
}

// 範囲は既定の値。範囲を変えて確かめるテストは、コマンドの Rules を差し替える
var defaultRules = studiorules.Defaults().Board

func (f fixture) registerCommand() *board.RegisterCommand {
	return &board.RegisterCommand{ID: f.id, PublicID: publicID, Name: f.name, Rules: defaultRules, RegisteredBy: f.operatorID}
}

func apply(t *testing.T, state board.State, cmd board.Command, now time.Time) (board.State, []board.Event) {
	t.Helper()
	events, err := board.Decide(state, cmd, now)
	require.NoError(t, err)
	for _, event := range events {
		state, err = board.Evolve(state, event)
		require.NoError(t, err)
	}
	return state, events
}

func (f fixture) register(t *testing.T) board.State {
	t.Helper()
	state, _ := apply(t, board.InitialState(), f.registerCommand(), registeredAt)
	return state
}

func (f fixture) changeLayoutCommand(layout board.Layout) *board.ChangeLayoutCommand {
	return &board.ChangeLayoutCommand{Layout: layout, Rules: defaultRules, ChangedBy: f.operatorID}
}

func (f fixture) changeLayout(t *testing.T, state board.State, layout board.Layout) board.State {
	t.Helper()
	state, _ = apply(t, state, f.changeLayoutCommand(layout), changedAt)
	return state
}

func (f fixture) publishCommand() *board.PublishCommand {
	return &board.PublishCommand{PublishedBy: f.operatorID}
}

func (f fixture) retireCommand() *board.RetireCommand {
	return &board.RetireCommand{RetiredBy: f.operatorID}
}

// 拠点 → 土地 → チェックポイントが横に並ぶ、公開できる 3×3
func publishableLayout(t *testing.T) board.Layout {
	t.Helper()
	return boardtest.Layout(t, boardtest.Size(t, 3, 3),
		[]board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "land", "fire"), boardtest.Tile(t, 2, 0, "checkpoint", "")},
		[]board.Connection{boardtest.Connection(0, 0, 1, 0), boardtest.Connection(1, 0, 2, 0)})
}

func (f fixture) publishableDraft(t *testing.T) board.State {
	t.Helper()
	return f.changeLayout(t, f.register(t), publishableLayout(t))
}

func (f fixture) published(t *testing.T) board.State {
	t.Helper()
	state, _ := apply(t, f.publishableDraft(t), f.publishCommand(), changedAt)
	return state
}

func (f fixture) retired(t *testing.T) board.State {
	t.Helper()
	state, _ := apply(t, f.published(t), f.retireCommand(), changedAt)
	return state
}

func TestDecide_RegisterMakesEmptyDraftOfDefaultSize(t *testing.T) {
	f := newFixture(t)

	events, err := board.Decide(board.InitialState(), f.registerCommand(), registeredAt)

	require.NoError(t, err)
	require.Equal(t, []board.Event{&board.RegisteredEvent{
		EventMeta:    board.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:     publicID,
		Name:         f.name,
		Width:        8,
		Height:       8,
		RegisteredBy: f.operatorID,
	}}, events)
	require.Equal(t, board.Board{
		ID: f.id, Version: 1, PublicID: publicID, Name: f.name, Status: board.StatusDraft,
		Layout: board.EmptyLayout(boardtest.Size(t, 8, 8)), RegisteredBy: f.operatorID, RegisteredAt: registeredAt, UpdatedAt: registeredAt,
	}, f.register(t))
}

// 既定の 8×8 が範囲の外なら、範囲の中に収める
func TestDecide_RegisterClampsDefaultSizeIntoRange(t *testing.T) {
	tests := []struct {
		name  string
		rules studiorules.Board
		want  int
	}{
		{name: "inside_range", rules: studiorulestest.Board(t, 1, 32), want: 8},
		{name: "min_above_default", rules: studiorulestest.Board(t, 10, 32), want: 10},
		{name: "max_below_default", rules: studiorulestest.Board(t, 1, 5), want: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cmd := f.registerCommand()
			cmd.Rules = tt.rules

			state, _ := apply(t, board.InitialState(), cmd, registeredAt)

			size := state.(board.Board).Layout.Size()
			require.Equal(t, tt.want, size.Width())
			require.Equal(t, tt.want, size.Height())
		})
	}
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)

	_, err := board.Decide(f.register(t), f.registerCommand(), registeredAt)

	require.ErrorIs(t, err, board.ErrInvalidState)
}

func TestEvolve_KeepsRegisteredSizeOutsideCurrentRange(t *testing.T) {
	f := newFixture(t)
	event := &board.RegisteredEvent{
		EventMeta: board.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:  publicID, Name: f.name, Width: defaultRules.MaxSize + 1, Height: 8, RegisteredBy: f.operatorID,
	}

	state, err := board.Evolve(board.InitialState(), event)

	require.NoError(t, err)
	require.Equal(t, defaultRules.MaxSize+1, state.(board.Board).Layout.Size().Width())
}

func TestDecide_RenameChangesNameWithNextVersion(t *testing.T) {
	f := newFixture(t)
	newName := boardtest.Name(t, "砂漠")

	state, events := apply(t, f.register(t), &board.RenameCommand{Name: newName, RenamedBy: f.operatorID}, changedAt)

	require.Equal(t, []board.Event{&board.RenamedEvent{
		EventMeta: board.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Name:      newName,
		RenamedBy: f.operatorID,
	}}, events)
	b := state.(board.Board)
	require.Equal(t, newName, b.Name)
	require.Equal(t, int64(2), b.Version)
	require.Equal(t, changedAt, b.UpdatedAt)
}

// 版を進めないため（画面で見た版を古くしないため）
func TestDecide_EmitsNothingWhenNothingChanges(t *testing.T) {
	f := newFixture(t)
	withLayout := f.publishableDraft(t)
	tests := []struct {
		name  string
		state board.State
		cmd   board.Command
	}{
		{name: "same_name", state: withLayout, cmd: &board.RenameCommand{Name: f.name, RenamedBy: f.operatorID}},
		{name: "same_layout", state: withLayout, cmd: f.changeLayoutCommand(publishableLayout(t))},
		{name: "same_layout_built_in_other_order", state: withLayout, cmd: f.changeLayoutCommand(boardtest.Layout(t, boardtest.Size(t, 3, 3),
			[]board.Tile{boardtest.Tile(t, 2, 0, "checkpoint", ""), boardtest.Tile(t, 1, 0, "land", "fire"), boardtest.Tile(t, 0, 0, "base", "")},
			[]board.Connection{boardtest.Connection(2, 0, 1, 0), boardtest.Connection(1, 0, 0, 0)}))},
		{name: "empty_layout_of_same_size", state: f.register(t), cmd: f.changeLayoutCommand(board.EmptyLayout(boardtest.Size(t, 8, 8)))},
		{name: "publish_published", state: f.published(t), cmd: f.publishCommand()},
		{name: "retire_retired", state: f.retired(t), cmd: f.retireCommand()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := board.Decide(tt.state, tt.cmd, changedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

func TestDecide_ChangeLayoutEmitsDifferenceInOrder(t *testing.T) {
	f := newFixture(t)
	before := boardtest.Layout(t, boardtest.Size(t, 3, 3),
		[]board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "land", "fire"), boardtest.Tile(t, 2, 0, "land", "fire"), boardtest.Tile(t, 0, 1, "land", "fire")},
		[]board.Connection{boardtest.Connection(0, 0, 1, 0), boardtest.Connection(1, 0, 2, 0), boardtest.Connection(0, 0, 0, 1)})
	// (2,0) を消して (2,1)・(1,1) を置き、(1,0) をチェックポイントにし、(0,1)-(1,1) をつなぐ。(0,0)-(0,1) は外す
	after := boardtest.Layout(t, boardtest.Size(t, 4, 3),
		[]board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", ""), boardtest.Tile(t, 0, 1, "land", "fire"), boardtest.Tile(t, 2, 1, "land", "fire"), boardtest.Tile(t, 1, 1, "land", "fire")},
		[]board.Connection{boardtest.Connection(0, 0, 1, 0), boardtest.Connection(1, 1, 2, 1), boardtest.Connection(0, 1, 1, 1)})
	size := boardtest.Size(t, 4, 3)

	state, events := apply(t, f.changeLayout(t, f.register(t), before), f.changeLayoutCommand(after), changedAt)

	require.Equal(t, []board.Event{&board.LayoutChangedEvent{
		EventMeta:    board.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Size:         &size,
		PlacedTiles:  []board.Tile{boardtest.Tile(t, 1, 1, "land", "fire"), boardtest.Tile(t, 2, 1, "land", "fire")},
		RemovedTiles: []board.Position{boardtest.Position(2, 0)},
		ChangedTiles: []board.Tile{boardtest.Tile(t, 1, 0, "checkpoint", "")},
		Connected:    []board.Connection{boardtest.Connection(0, 1, 1, 1), boardtest.Connection(1, 1, 2, 1)},
		Disconnected: []board.Connection{boardtest.Connection(0, 0, 0, 1), boardtest.Connection(1, 0, 2, 0)},
		ChangedBy:    f.operatorID,
	}}, events)
	require.Equal(t, after, state.(board.Board).Layout)
}

func TestDecide_ChangeLayoutEmitsChangedTileWhenOnlyAttributeChanges(t *testing.T) {
	f := newFixture(t)
	size := boardtest.Size(t, 2, 1)
	before := boardtest.Layout(t, size, []board.Tile{boardtest.Tile(t, 0, 0, "land", "fire"), boardtest.Tile(t, 1, 0, "land", "water")}, nil)
	after := boardtest.Layout(t, size, []board.Tile{boardtest.Tile(t, 0, 0, "land", "fire"), boardtest.Tile(t, 1, 0, "land", "multi")}, nil)

	_, events := apply(t, f.changeLayout(t, f.register(t), before), f.changeLayoutCommand(after), changedAt)

	require.Len(t, events, 1)
	event := events[0].(*board.LayoutChangedEvent)
	require.Equal(t, []board.Tile{boardtest.Tile(t, 1, 0, "land", "multi")}, event.ChangedTiles)
	require.Empty(t, event.PlacedTiles)
	require.Empty(t, event.RemovedTiles)
}

func TestDecide_RejectsChangeLayoutOutsideRange(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		rules         studiorules.Board
	}{
		{name: "too_wide", width: 33, height: 8, rules: defaultRules},
		{name: "too_high", width: 8, height: 33, rules: defaultRules},
		{name: "below_min_width", width: 3, height: 8, rules: studiorulestest.Board(t, 4, 32)},
		{name: "above_lowered_max_height", width: 8, height: 8, rules: studiorulestest.Board(t, 1, 7)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cmd := f.changeLayoutCommand(board.EmptyLayout(boardtest.Size(t, tt.width, tt.height)))
			cmd.Rules = tt.rules

			_, err := board.Decide(f.register(t), cmd, changedAt)

			require.ErrorIs(t, err, board.ErrSizeOutOfRange)
		})
	}
}

// 範囲を下げた後は、大きさを変えない保存でも新しい範囲で確かめる（準備中のものを保存し直すときに範囲に収めさせるため）
func TestDecide_RejectsChangeLayoutKeepingSizeOutsideLoweredRange(t *testing.T) {
	f := newFixture(t)
	cmd := f.changeLayoutCommand(board.EmptyLayout(boardtest.Size(t, 8, 8)))
	cmd.Rules = studiorulestest.Board(t, 1, 7)

	_, err := board.Decide(f.register(t), cmd, changedAt)

	require.ErrorIs(t, err, board.ErrSizeOutOfRange)
}

func TestDecide_ChangeLayoutOmitsSizeWhenSizeIsSame(t *testing.T) {
	f := newFixture(t)
	layout := boardtest.Layout(t, boardtest.Size(t, 8, 8), []board.Tile{boardtest.Tile(t, 7, 7, "land", "fire")}, nil)

	_, events := apply(t, f.register(t), f.changeLayoutCommand(layout), changedAt)

	event := events[0].(*board.LayoutChangedEvent)
	require.Nil(t, event.Size)
	require.Equal(t, []board.Tile{boardtest.Tile(t, 7, 7, "land", "fire")}, event.PlacedTiles)
}

// 大きさだけを変えたときも、マスの差分は空でイベントを作る
func TestDecide_ChangeLayoutEmitsSizeAloneWhenOnlySizeChanges(t *testing.T) {
	f := newFixture(t)
	size := boardtest.Size(t, 5, 6)

	state, events := apply(t, f.register(t), f.changeLayoutCommand(board.EmptyLayout(size)), changedAt)

	require.Equal(t, []board.Event{&board.LayoutChangedEvent{
		EventMeta: board.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Size:      &size,
		ChangedBy: f.operatorID,
	}}, events)
	require.Equal(t, size, state.(board.Board).Layout.Size())
}

// マスを消すと、そのマスのつながりも切れる（前と後の集合の差なので Disconnected に入る）
func TestDecide_ChangeLayoutDisconnectsConnectionsOfRemovedTile(t *testing.T) {
	f := newFixture(t)
	after := boardtest.Layout(t, boardtest.Size(t, 3, 3), []board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "land", "fire")},
		[]board.Connection{boardtest.Connection(0, 0, 1, 0)})

	_, events := apply(t, f.publishableDraft(t), f.changeLayoutCommand(after), changedAt)

	event := events[0].(*board.LayoutChangedEvent)
	require.Equal(t, []board.Position{boardtest.Position(2, 0)}, event.RemovedTiles)
	require.Equal(t, []board.Connection{boardtest.Connection(1, 0, 2, 0)}, event.Disconnected)
	require.Empty(t, event.Connected)
}

func TestEvolve_DoesNotChangeTheGivenState(t *testing.T) {
	f := newFixture(t)
	before := f.publishableDraft(t)

	f.changeLayout(t, before, board.EmptyLayout(boardtest.Size(t, 2, 2)))

	require.Equal(t, publishableLayout(t), before.(board.Board).Layout)
}

func TestEvolve_RejectsLayoutChangeThatBreaksLayoutRules(t *testing.T) {
	f := newFixture(t)
	event := &board.LayoutChangedEvent{
		EventMeta:    board.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		RemovedTiles: []board.Position{boardtest.Position(2, 0)},
		ChangedBy:    f.operatorID,
	}

	_, err := board.Evolve(f.publishableDraft(t), event)

	require.ErrorIs(t, err, board.ErrInvalidLayout)
}

func TestDecide_PublishMakesPublished(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.publishableDraft(t), f.publishCommand(), changedAt)

	require.Equal(t, []board.Event{&board.PublishedEvent{
		EventMeta:   board.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		PublishedBy: f.operatorID,
	}}, events)
	require.Equal(t, board.StatusPublished, state.(board.Board).Status)
}

// 行き止まり（つながりが 1 本だけのマス）と、チェックポイントが複数あることは認める
func TestDecide_PublishAllowsDeadEndsAndManyCheckpoints(t *testing.T) {
	f := newFixture(t)
	layout := boardtest.Layout(t, boardtest.Size(t, 3, 3),
		[]board.Tile{boardtest.Tile(t, 1, 1, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", ""), boardtest.Tile(t, 0, 1, "checkpoint", ""), boardtest.Tile(t, 2, 1, "land", "fire")},
		[]board.Connection{boardtest.Connection(1, 1, 1, 0), boardtest.Connection(1, 1, 0, 1), boardtest.Connection(1, 1, 2, 1)})

	state, _ := apply(t, f.changeLayout(t, f.register(t), layout), f.publishCommand(), changedAt)

	require.Equal(t, board.StatusPublished, state.(board.Board).Status)
}

func TestDecide_RejectsPublishUnlessConditionsAreMet(t *testing.T) {
	f := newFixture(t)
	size := boardtest.Size(t, 3, 3)
	tests := []struct {
		name        string
		layout      board.Layout
		wantBases   int
		wantChecks  int
		wantUnreach []board.Position
	}{
		{name: "empty", layout: board.EmptyLayout(size)},
		{
			name:       "no_base",
			layout:     boardtest.Layout(t, size, []board.Tile{boardtest.Tile(t, 0, 0, "checkpoint", "")}, nil),
			wantChecks: 1,
		},
		{
			name:       "two_bases",
			layout:     boardtest.Layout(t, size, []board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "base", ""), boardtest.Tile(t, 2, 0, "checkpoint", "")}, nil),
			wantBases:  2,
			wantChecks: 1,
		},
		{
			name:      "no_checkpoint",
			layout:    boardtest.Layout(t, size, []board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "land", "fire")}, []board.Connection{boardtest.Connection(0, 0, 1, 0)}),
			wantBases: 1,
		},
		{
			name: "unreachable_tiles",
			layout: boardtest.Layout(t, size,
				[]board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", ""), boardtest.Tile(t, 2, 2, "land", "fire"), boardtest.Tile(t, 0, 2, "land", "fire"), boardtest.Tile(t, 1, 2, "checkpoint", "")},
				[]board.Connection{boardtest.Connection(0, 0, 1, 0), boardtest.Connection(0, 2, 1, 2)}),
			wantBases:   1,
			wantChecks:  2,
			wantUnreach: []board.Position{boardtest.Position(0, 2), boardtest.Position(1, 2), boardtest.Position(2, 2)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := board.Decide(f.changeLayout(t, f.register(t), tt.layout), f.publishCommand(), changedAt)

			require.ErrorIs(t, err, board.ErrNotPublishable)
			var notPublishable *board.NotPublishableError
			require.True(t, errors.As(err, &notPublishable))
			require.Equal(t, board.Readiness{BaseCount: tt.wantBases, CheckpointCount: tt.wantChecks, UnreachableTiles: tt.wantUnreach}, notPublishable.Readiness)
		})
	}
}

func TestDecide_RetiredCanBePublishedAgain(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.retired(t), f.publishCommand(), changedAt)

	require.Len(t, events, 1)
	require.Equal(t, board.StatusPublished, state.(board.Board).Status)
}

func TestDecide_RetireMakesRetiredOnlyFromPublished(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.published(t), f.retireCommand(), changedAt)

	require.Equal(t, []board.Event{&board.RetiredEvent{
		EventMeta: board.EventMeta{EntityID: f.id, AggregateVersion: 4, OccurredAt: changedAt},
		RetiredBy: f.operatorID,
	}}, events)
	require.Equal(t, board.StatusRetired, state.(board.Board).Status)

	_, err := board.Decide(f.register(t), f.retireCommand(), changedAt)
	require.ErrorIs(t, err, board.ErrInvalidStatusTransition)
}

// 使われている間にレイアウトが変わらないよう、準備中以外では名前・レイアウトを変えられず、消せない。準備中に戻す操作も無い
func TestDecide_RejectsEditAndDeleteUnlessDraft(t *testing.T) {
	f := newFixture(t)
	commands := []struct {
		name string
		cmd  board.Command
	}{
		{name: "rename", cmd: &board.RenameCommand{Name: boardtest.Name(t, "砂漠"), RenamedBy: f.operatorID}},
		{name: "change_layout", cmd: f.changeLayoutCommand(board.EmptyLayout(boardtest.Size(t, 2, 2)))},
		{name: "delete", cmd: &board.DeleteCommand{DeletedBy: f.operatorID}},
	}
	states := []struct {
		name  string
		state board.State
	}{
		{name: "published", state: f.published(t)},
		{name: "retired", state: f.retired(t)},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := board.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, board.ErrNotDraft)
			})
		}
	}
}

func TestDecide_DeleteMakesDeletedBoard(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.register(t), &board.DeleteCommand{DeletedBy: f.operatorID}, changedAt)

	require.Equal(t, []board.Event{&board.DeletedEvent{
		EventMeta: board.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		DeletedBy: f.operatorID,
	}}, events)
	require.Equal(t, board.DeletedBoard{ID: f.id, Version: 2}, state)
}

func TestDecide_RejectsChangesUnlessRegistered(t *testing.T) {
	f := newFixture(t)
	deleted, _ := apply(t, f.register(t), &board.DeleteCommand{DeletedBy: f.operatorID}, changedAt)
	commands := []struct {
		name string
		cmd  board.Command
	}{
		{name: "rename", cmd: &board.RenameCommand{Name: boardtest.Name(t, "砂漠"), RenamedBy: f.operatorID}},
		{name: "change_layout", cmd: f.changeLayoutCommand(board.EmptyLayout(boardtest.Size(t, 2, 2)))},
		{name: "publish", cmd: f.publishCommand()},
		{name: "retire", cmd: f.retireCommand()},
		{name: "delete", cmd: &board.DeleteCommand{DeletedBy: f.operatorID}},
	}
	states := []struct {
		name  string
		state board.State
	}{
		{name: "unregistered", state: board.InitialState()},
		{name: "deleted", state: deleted},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := board.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, board.ErrInvalidState)
			})
		}
	}
}

func TestStatus_TransitionsFollowRules(t *testing.T) {
	tests := []struct {
		name        string
		from        board.Status
		publishTo   board.Status
		publishDiff bool
		retireTo    board.Status
		retireDiff  bool
		retireErr   error
	}{
		{name: "draft", from: board.StatusDraft, publishTo: board.StatusPublished, publishDiff: true, retireTo: board.StatusDraft, retireErr: board.ErrInvalidStatusTransition},
		{name: "published", from: board.StatusPublished, publishTo: board.StatusPublished, retireTo: board.StatusRetired, retireDiff: true},
		{name: "retired", from: board.StatusRetired, publishTo: board.StatusPublished, publishDiff: true, retireTo: board.StatusRetired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			to, changed, err := tt.from.Publish()
			require.NoError(t, err)
			require.Equal(t, tt.publishTo, to)
			require.Equal(t, tt.publishDiff, changed)

			to, changed, err = tt.from.Retire()
			require.ErrorIs(t, err, tt.retireErr)
			require.Equal(t, tt.retireTo, to)
			require.Equal(t, tt.retireDiff, changed)
		})
	}
}

func TestNewStatus_RejectsUnknownValue(t *testing.T) {
	_, err := board.NewStatus("archived")

	require.ErrorIs(t, err, board.ErrStatusUnknown)
}

func TestNewTileKind_RejectsUnknownValue(t *testing.T) {
	for _, value := range []string{"land", "base", "checkpoint"} {
		kind, err := board.NewTileKind(value)
		require.NoError(t, err)
		require.Equal(t, value, kind.String())
	}

	_, err := board.NewTileKind("water")
	require.ErrorIs(t, err, board.ErrTileKindUnknown)
}

func TestNewAttribute_RejectsUnknownValue(t *testing.T) {
	for _, value := range []string{"fire", "earth", "water", "wind", "neutral", "multi"} {
		attribute, err := board.NewAttribute(value)
		require.NoError(t, err)
		require.Equal(t, value, attribute.String())
	}

	_, err := board.NewAttribute("light")

	require.ErrorIs(t, err, board.ErrAttributeUnknown)
}

func TestNewName_RejectsBlankAndTooLong(t *testing.T) {
	name, err := board.NewName("  草原  ")
	require.NoError(t, err)
	require.Equal(t, "草原", name.String())
	_, err = board.NewName(strings.Repeat("あ", board.MaxNameLength))
	require.NoError(t, err)

	_, err = board.NewName("   ")
	require.ErrorIs(t, err, board.ErrNameEmpty)
	_, err = board.NewName(strings.Repeat("あ", board.MaxNameLength+1))
	require.ErrorIs(t, err, board.ErrNameTooLong)
}

// 範囲（上限・下限）は studio_rules が決めるので、値の形（1 以上）だけを確かめる
func TestNewSize_AcceptsOneOrMore(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		wantErr       bool
	}{
		{name: "minimum", width: 1, height: 1},
		{name: "beyond_default_max", width: 100, height: 33},
		{name: "not_square", width: 1, height: 32},
		{name: "zero_width", width: 0, height: 8, wantErr: true},
		{name: "zero_height", width: 8, height: 0, wantErr: true},
		{name: "negative_width", width: -1, height: 8, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size, err := board.NewSize(tt.width, tt.height)

			if tt.wantErr {
				require.ErrorIs(t, err, board.ErrSizeOutOfRange)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.width, size.Width())
			require.Equal(t, tt.height, size.Height())
		})
	}
}

func TestValidateLayout_ReportsEachViolationWithItsPlace(t *testing.T) {
	size := boardtest.Size(t, 3, 2)
	land := func(x, y int) board.Tile { return boardtest.Tile(t, x, y, "land", "fire") }
	tests := []struct {
		name        string
		tiles       []board.Tile
		connections []board.Connection
		want        []board.LayoutViolation
	}{
		{name: "valid", tiles: []board.Tile{land(0, 0), land(2, 1)}},
		{
			name:  "land_without_attribute",
			tiles: []board.Tile{land(0, 0), {Position: boardtest.Position(1, 0), Kind: boardtest.TileKind(t, "land")}},
			want:  []board.LayoutViolation{{Subject: board.SubjectTile, Index: 1, Part: board.PartAttribute, Reason: board.ReasonAttributeRequired}},
		},
		{
			name:  "base_with_attribute",
			tiles: []board.Tile{{Position: boardtest.Position(0, 0), Kind: boardtest.TileKind(t, "base"), Attribute: boardtest.Attribute(t, "fire")}},
			want:  []board.LayoutViolation{{Subject: board.SubjectTile, Index: 0, Part: board.PartAttribute, Reason: board.ReasonAttributeNotAllowed}},
		},
		{
			name:  "checkpoint_with_attribute",
			tiles: []board.Tile{{Position: boardtest.Position(0, 0), Kind: boardtest.TileKind(t, "checkpoint"), Attribute: boardtest.Attribute(t, "multi")}},
			want:  []board.LayoutViolation{{Subject: board.SubjectTile, Index: 0, Part: board.PartAttribute, Reason: board.ReasonAttributeNotAllowed}},
		},
		{
			name:  "out_of_range_land_without_attribute",
			tiles: []board.Tile{{Position: boardtest.Position(3, 0), Kind: boardtest.TileKind(t, "land")}},
			want: []board.LayoutViolation{
				{Subject: board.SubjectTile, Index: 0, Part: board.PartX, Reason: board.ReasonOutOfRange},
				{Subject: board.SubjectTile, Index: 0, Part: board.PartAttribute, Reason: board.ReasonAttributeRequired},
			},
		},
		{
			name:  "x_out_of_range",
			tiles: []board.Tile{land(0, 0), land(3, 0)},
			want:  []board.LayoutViolation{{Subject: board.SubjectTile, Index: 1, Part: board.PartX, Reason: board.ReasonOutOfRange}},
		},
		{
			name:  "negative_y",
			tiles: []board.Tile{land(0, -1)},
			want:  []board.LayoutViolation{{Subject: board.SubjectTile, Index: 0, Part: board.PartY, Reason: board.ReasonOutOfRange}},
		},
		{
			name:  "x_and_y_out_of_range",
			tiles: []board.Tile{land(3, 2)},
			want: []board.LayoutViolation{
				{Subject: board.SubjectTile, Index: 0, Part: board.PartX, Reason: board.ReasonOutOfRange},
				{Subject: board.SubjectTile, Index: 0, Part: board.PartY, Reason: board.ReasonOutOfRange},
			},
		},
		{
			name:  "duplicated_position",
			tiles: []board.Tile{land(1, 1), boardtest.Tile(t, 1, 1, "base", ""), land(1, 1)},
			want: []board.LayoutViolation{
				{Subject: board.SubjectTile, Index: 1, Part: board.PartWhole, Reason: board.ReasonDuplicatedTile},
				{Subject: board.SubjectTile, Index: 2, Part: board.PartWhole, Reason: board.ReasonDuplicatedTile},
			},
		},
		{
			name:        "end_a_without_tile",
			tiles:       []board.Tile{land(1, 0)},
			connections: []board.Connection{boardtest.Connection(0, 0, 1, 0)},
			want:        []board.LayoutViolation{{Subject: board.SubjectConnection, Index: 0, Part: board.PartA, Reason: board.ReasonTileNotFound}},
		},
		{
			name:        "end_b_without_tile",
			tiles:       []board.Tile{land(0, 0)},
			connections: []board.Connection{boardtest.Connection(0, 0, 1, 0)},
			want:        []board.LayoutViolation{{Subject: board.SubjectConnection, Index: 0, Part: board.PartB, Reason: board.ReasonTileNotFound}},
		},
		{
			name:        "ends_outside_the_grid_without_tiles",
			connections: []board.Connection{boardtest.Connection(5, 5, 5, 6)},
			want: []board.LayoutViolation{
				{Subject: board.SubjectConnection, Index: 0, Part: board.PartA, Reason: board.ReasonTileNotFound},
				{Subject: board.SubjectConnection, Index: 0, Part: board.PartB, Reason: board.ReasonTileNotFound},
			},
		},
		{
			name:        "diagonal",
			tiles:       []board.Tile{land(0, 0), land(1, 1)},
			connections: []board.Connection{boardtest.Connection(0, 0, 1, 1)},
			want:        []board.LayoutViolation{{Subject: board.SubjectConnection, Index: 0, Part: board.PartWhole, Reason: board.ReasonNotAdjacent}},
		},
		{
			name:        "two_apart",
			tiles:       []board.Tile{land(0, 0), land(2, 0)},
			connections: []board.Connection{boardtest.Connection(0, 0, 2, 0)},
			want:        []board.LayoutViolation{{Subject: board.SubjectConnection, Index: 0, Part: board.PartWhole, Reason: board.ReasonNotAdjacent}},
		},
		{
			name:        "same_position_at_both_ends",
			tiles:       []board.Tile{land(0, 0)},
			connections: []board.Connection{boardtest.Connection(0, 0, 0, 0)},
			want:        []board.LayoutViolation{{Subject: board.SubjectConnection, Index: 0, Part: board.PartWhole, Reason: board.ReasonNotAdjacent}},
		},
		{
			name:        "duplicated_connection_in_reverse_order",
			tiles:       []board.Tile{land(0, 0), land(1, 0)},
			connections: []board.Connection{boardtest.Connection(0, 0, 1, 0), boardtest.Connection(1, 0, 0, 0)},
			want:        []board.LayoutViolation{{Subject: board.SubjectConnection, Index: 1, Part: board.PartWhole, Reason: board.ReasonDuplicatedConnection}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, board.ValidateLayout(size, tt.tiles, tt.connections))
		})
	}
}

func TestNewLayout_RejectsViolationsWithTheirList(t *testing.T) {
	size := boardtest.Size(t, 2, 2)

	_, err := board.NewLayout(size, []board.Tile{boardtest.Tile(t, 2, 0, "land", "fire")}, nil)

	require.ErrorIs(t, err, board.ErrInvalidLayout)
	var invalid *board.InvalidLayoutError
	require.True(t, errors.As(err, &invalid))
	require.Equal(t, []board.LayoutViolation{{Subject: board.SubjectTile, Index: 0, Part: board.PartX, Reason: board.ReasonOutOfRange}}, invalid.Violations)
}

func TestLayout_ListsTilesByRowThenColumnAndConnectionsFromA(t *testing.T) {
	layout := boardtest.Layout(t, boardtest.Size(t, 3, 3),
		[]board.Tile{boardtest.Tile(t, 0, 1, "land", "fire"), boardtest.Tile(t, 2, 0, "land", "fire"), boardtest.Tile(t, 1, 0, "land", "fire"), boardtest.Tile(t, 1, 1, "land", "fire")},
		[]board.Connection{boardtest.Connection(1, 1, 1, 0), boardtest.Connection(1, 0, 2, 0), boardtest.Connection(0, 1, 1, 1)})

	require.Equal(t, []board.Tile{
		boardtest.Tile(t, 1, 0, "land", "fire"), boardtest.Tile(t, 2, 0, "land", "fire"), boardtest.Tile(t, 0, 1, "land", "fire"), boardtest.Tile(t, 1, 1, "land", "fire"),
	}, layout.Tiles())
	require.Equal(t, []board.Connection{
		boardtest.Connection(1, 0, 2, 0), boardtest.Connection(1, 0, 1, 1), boardtest.Connection(0, 1, 1, 1),
	}, layout.Connections())
}

func TestNewConnection_PutsSmallerPositionFirst(t *testing.T) {
	connection := boardtest.Connection(2, 1, 2, 0)

	require.Equal(t, boardtest.Position(2, 0), connection.A())
	require.Equal(t, boardtest.Position(2, 1), connection.B())
	require.Equal(t, connection, boardtest.Connection(2, 0, 2, 1))
}

func TestReadiness_LeavesUnreachableEmptyUnlessExactlyOneBase(t *testing.T) {
	size := boardtest.Size(t, 3, 3)
	layout := boardtest.Layout(t, size,
		[]board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "base", ""), boardtest.Tile(t, 2, 2, "land", "fire")}, nil)

	require.Equal(t, board.Readiness{BaseCount: 2}, layout.Readiness())
}

func TestReadiness_TracesConnectionsInEitherDirection(t *testing.T) {
	size := boardtest.Size(t, 3, 3)
	layout := boardtest.Layout(t, size,
		[]board.Tile{boardtest.Tile(t, 2, 2, "base", ""), boardtest.Tile(t, 1, 2, "land", "fire"), boardtest.Tile(t, 0, 2, "checkpoint", "")},
		[]board.Connection{boardtest.Connection(0, 2, 1, 2), boardtest.Connection(1, 2, 2, 2)})

	readiness := layout.Readiness()

	require.Equal(t, board.Readiness{BaseCount: 1, CheckpointCount: 1}, readiness)
	require.True(t, readiness.Publishable())
}

// 公開中の名前だけでなく、状態によらず、すべてのボードで比べる。比べ方はカード名と同じ
func TestNameKey_TreatsWidthAndCaseAsSame(t *testing.T) {
	require.Equal(t,
		board.NameKey(boardtest.Name(t, "Ｆire 草原")),
		board.NameKey(boardtest.Name(t, "fire 草原")))
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	f := newFixture(t)
	meta := board.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt}
	size := boardtest.Size(t, 4, 3)
	events := []board.Event{
		&board.RegisteredEvent{
			EventMeta: meta, PublicID: publicID, Name: f.name, Width: 8, Height: 8, RegisteredBy: f.operatorID,
		},
		&board.RenamedEvent{EventMeta: meta, Name: boardtest.Name(t, "砂漠"), RenamedBy: f.operatorID},
		&board.LayoutChangedEvent{
			EventMeta:    meta,
			Size:         &size,
			PlacedTiles:  []board.Tile{boardtest.Tile(t, 1, 0, "land", "fire"), boardtest.Tile(t, 0, 1, "base", "")},
			RemovedTiles: []board.Position{boardtest.Position(2, 1)},
			ChangedTiles: []board.Tile{boardtest.Tile(t, 3, 2, "checkpoint", ""), boardtest.Tile(t, 3, 1, "land", "multi")},
			Connected:    []board.Connection{boardtest.Connection(0, 1, 1, 1)},
			Disconnected: []board.Connection{boardtest.Connection(1, 1, 2, 1)},
			ChangedBy:    f.operatorID,
		},
		&board.PublishedEvent{EventMeta: meta, PublishedBy: f.operatorID},
		&board.RetiredEvent{EventMeta: meta, RetiredBy: f.operatorID},
		&board.DeletedEvent{EventMeta: meta, DeletedBy: f.operatorID},
	}
	for _, event := range events {
		t.Run(event.EventType().String(), func(t *testing.T) {
			require.Equal(t, 1, event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := board.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, event, restored)
		})
	}
}

// 変わらなかった部分は payload に出さない
func TestEvent_LayoutChangedOmitsEmptyLists(t *testing.T) {
	f := newFixture(t)
	event := &board.LayoutChangedEvent{
		EventMeta:   board.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		PlacedTiles: []board.Tile{boardtest.Tile(t, 1, 0, "land", "fire")},
		ChangedBy:   f.operatorID,
	}

	payload, err := json.Marshal(event)
	require.NoError(t, err)

	require.JSONEq(t, `{"placed_tiles":[{"x":1,"y":0,"kind":"land","attribute":"fire"}],"changed_by":10}`, string(payload))
	restored, err := board.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), 1, payload)
	require.NoError(t, err)
	require.Equal(t, event, restored)
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := board.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(board.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}
