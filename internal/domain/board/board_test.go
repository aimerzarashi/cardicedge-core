package board_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
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
	return &board.ChangeLayoutCommand{Layout: layout, ChangedBy: f.operatorID}
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
		[]board.Connection{boardtest.Connection(t, 0, 0, 1, 0), boardtest.Connection(t, 1, 0, 2, 0)})
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

func initialDefaultRules(t *testing.T) board.DefaultRules {
	t.Helper()
	return boardtest.DefaultRules(t, 8000, 600, 300, 100, 30, 1, 6)
}

func (f fixture) changeDefaultRulesCommand(rules board.DefaultRules) *board.ChangeDefaultRulesCommand {
	return &board.ChangeDefaultRulesCommand{DefaultRules: rules, ChangedBy: f.operatorID}
}

func TestDecide_RegisterMakesEmptyDraftOfDefaultSize(t *testing.T) {
	f := newFixture(t)

	events, err := board.Decide(board.InitialState(), f.registerCommand(), registeredAt)

	require.NoError(t, err)
	require.Equal(t, []board.Event{&board.RegisteredEvent{
		EventMeta:    board.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:     publicID,
		Name:         f.name,
		Width:        boardtest.Width(t, 8),
		Height:       boardtest.Height(t, 8),
		DefaultRules: initialDefaultRules(t),
		RegisteredBy: f.operatorID,
	}}, events)
	require.Equal(t, board.Board{
		ID: f.id, Version: 1, PublicID: publicID, Name: f.name, Status: board.StatusDraft,
		Layout: board.EmptyLayout(boardtest.Size(t, 8, 8)), DefaultRules: initialDefaultRules(t),
	}, f.register(t))
}

// 既定の 8×8 が範囲の外なら、範囲の中に収める
func TestDecide_RegisterClampsDefaultSizeIntoRange(t *testing.T) {
	tests := []struct {
		name  string
		rules board.SizeRange
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
			require.Equal(t, tt.want, size.Width.Int())
			require.Equal(t, tt.want, size.Height.Int())
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
		PublicID:  publicID, Name: f.name, Width: boardtest.Width(t, defaultRules.MaxSize.Int()+1), Height: boardtest.Height(t, 8), DefaultRules: initialDefaultRules(t), RegisteredBy: f.operatorID,
	}

	state, err := board.Evolve(board.InitialState(), event)

	require.NoError(t, err)
	require.Equal(t, defaultRules.MaxSize.Int()+1, state.(board.Board).Layout.Size().Width.Int())
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
}

func TestDecide_ChangeDefaultRulesReplacesAllValues(t *testing.T) {
	f := newFixture(t)
	rules := boardtest.DefaultRules(t, 5000, 0, 200, 50, 12, 2, 8)

	state, events := apply(t, f.register(t), f.changeDefaultRulesCommand(rules), changedAt)

	require.Equal(t, []board.Event{&board.DefaultRulesChangedEvent{
		EventMeta:    board.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		DefaultRules: rules,
		ChangedBy:    f.operatorID,
	}}, events)
	b := state.(board.Board)
	require.Equal(t, rules, b.DefaultRules)
	require.Equal(t, int64(2), b.Version)
}

// 値を決めたのは登録のときで、定数を変えても過去のボードの値が変わらないことを、登録のイベントの値で確かめる
func TestEvolve_KeepsRegisteredDefaultRules(t *testing.T) {
	f := newFixture(t)
	rules := boardtest.DefaultRules(t, 9999, 1, 2, 3, 4, 5, 6)
	event := &board.RegisteredEvent{
		EventMeta: board.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:  publicID, Name: f.name, Width: boardtest.Width(t, 8), Height: boardtest.Height(t, 8), DefaultRules: rules, RegisteredBy: f.operatorID,
	}

	state, err := board.Evolve(board.InitialState(), event)

	require.NoError(t, err)
	require.Equal(t, rules, state.(board.Board).DefaultRules)
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
			[]board.Connection{boardtest.Connection(t, 2, 0, 1, 0), boardtest.Connection(t, 1, 0, 0, 0)}))},
		{name: "same_default_rules", state: withLayout, cmd: f.changeDefaultRulesCommand(initialDefaultRules(t))},
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
		[]board.Connection{boardtest.Connection(t, 0, 0, 1, 0), boardtest.Connection(t, 1, 0, 2, 0), boardtest.Connection(t, 0, 0, 0, 1)})
	// (2,0) を消して (2,1)・(1,1) を置き、(1,0) をチェックポイントにし、(0,1)-(1,1) をつなぐ。(0,0)-(0,1) は外す
	after := boardtest.Layout(t, boardtest.Size(t, 4, 3),
		[]board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", ""), boardtest.Tile(t, 0, 1, "land", "fire"), boardtest.Tile(t, 2, 1, "land", "fire"), boardtest.Tile(t, 1, 1, "land", "fire")},
		[]board.Connection{boardtest.Connection(t, 0, 0, 1, 0), boardtest.Connection(t, 1, 1, 2, 1), boardtest.Connection(t, 0, 1, 1, 1)})
	size := boardtest.Size(t, 4, 3)

	state, events := apply(t, f.changeLayout(t, f.register(t), before), f.changeLayoutCommand(after), changedAt)

	require.Equal(t, []board.Event{&board.LayoutChangedEvent{
		EventMeta:    board.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Size:         &size,
		PlacedTiles:  []board.Tile{boardtest.Tile(t, 1, 1, "land", "fire"), boardtest.Tile(t, 2, 1, "land", "fire")},
		RemovedTiles: []board.Position{boardtest.Position(t, 2, 0)},
		ChangedTiles: []board.Tile{boardtest.Tile(t, 1, 0, "checkpoint", "")},
		Connected:    []board.Connection{boardtest.Connection(t, 0, 1, 1, 1), boardtest.Connection(t, 1, 1, 2, 1)},
		Disconnected: []board.Connection{boardtest.Connection(t, 0, 0, 0, 1), boardtest.Connection(t, 1, 0, 2, 0)},
		ChangedBy:    f.operatorID,
	}}, events)
	require.Equal(t, after, state.(board.Board).Layout)
}

func TestDecide_ChangeLayoutEmitsChangedTileWhenOnlyElementChanges(t *testing.T) {
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

func TestNewWidthAndHeight_RejectSizeOutsideRange(t *testing.T) {
	tests := []struct {
		name  string
		value int
		rules board.SizeRange
	}{
		{name: "above_max", value: 33, rules: defaultRules},
		{name: "zero", value: 0, rules: defaultRules},
		{name: "below_min", value: 3, rules: boardtest.SizeRange(t, 4, 32)},
		{name: "above_lowered_max", value: 8, rules: boardtest.SizeRange(t, 1, 7)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := board.NewWidth(tt.value, tt.rules)
			require.ErrorIs(t, err, board.ErrSizeOutOfRange)

			_, err = board.NewHeight(tt.value, tt.rules)
			require.ErrorIs(t, err, board.ErrSizeOutOfRange)
		})
	}
}

func TestNewWidthAndHeight_AcceptSizeAtRangeEnds(t *testing.T) {
	rules := boardtest.SizeRange(t, 4, 10)
	for _, value := range []int{4, 10} {
		width, err := board.NewWidth(value, rules)
		require.NoError(t, err)
		require.Equal(t, value, width.Int())

		height, err := board.NewHeight(value, rules)
		require.NoError(t, err)
		require.Equal(t, value, height.Int())
	}
}

func TestNewSizeRange_RejectsInvalidRange(t *testing.T) {
	tests := []struct {
		name             string
		minSize, maxSize int
	}{
		{name: "zero_min_size", minSize: 0, maxSize: 32},
		{name: "negative_min_size", minSize: -1, maxSize: 32},
		{name: "zero_max_size", minSize: 1, maxSize: 0},
		{name: "max_size_below_min_size", minSize: 5, maxSize: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minSize, minErr := board.NewMinSize(tt.minSize)
			maxSize, maxErr := board.NewMaxSize(tt.maxSize)
			if minErr != nil || maxErr != nil {
				require.ErrorIs(t, errors.Join(minErr, maxErr), board.ErrSizeRangeInvalid)
				return
			}

			_, err := board.NewSizeRange(minSize, maxSize)

			require.ErrorIs(t, err, board.ErrSizeRangeInvalid)
		})
	}
}

func TestNewSizeRange_AcceptsSingleSizeRange(t *testing.T) {
	_ = boardtest.SizeRange(t, 8, 8)
}

func TestSizeRange_ContainsAndClamp(t *testing.T) {
	rules := boardtest.SizeRange(t, 4, 10)
	tests := []struct {
		name      string
		size      int
		contained bool
		clamped   int
	}{
		{name: "below_min", size: 3, contained: false, clamped: 4},
		{name: "min", size: 4, contained: true, clamped: 4},
		{name: "inside", size: 8, contained: true, clamped: 8},
		{name: "max", size: 10, contained: true, clamped: 10},
		{name: "above_max", size: 11, contained: false, clamped: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.contained, rules.Contains(tt.size))
			require.Equal(t, tt.clamped, rules.Clamp(tt.size))
		})
	}
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
		[]board.Connection{boardtest.Connection(t, 0, 0, 1, 0)})

	_, events := apply(t, f.publishableDraft(t), f.changeLayoutCommand(after), changedAt)

	event := events[0].(*board.LayoutChangedEvent)
	require.Equal(t, []board.Position{boardtest.Position(t, 2, 0)}, event.RemovedTiles)
	require.Equal(t, []board.Connection{boardtest.Connection(t, 1, 0, 2, 0)}, event.Disconnected)
	require.Empty(t, event.Connected)
}

func TestDecide_ChangeLayoutEmitsPairDifferenceInOrder(t *testing.T) {
	f := newFixture(t)
	size := boardtest.Size(t, 4, 2)
	tiles := []board.Tile{
		boardtest.Tile(t, 0, 0, "warp", ""), boardtest.Tile(t, 3, 0, "warp", ""), boardtest.Tile(t, 0, 1, "warp", ""), boardtest.Tile(t, 3, 1, "warp", ""),
		boardtest.Tile(t, 1, 1, "bridge", ""), boardtest.Tile(t, 2, 1, "bridge", ""),
	}
	before := boardtest.Layout(t, size, tiles, nil, boardtest.Pair(t, 0, 0, 3, 0), boardtest.Pair(t, 1, 1, 2, 1))
	after := boardtest.Layout(t, size, tiles, nil, boardtest.Pair(t, 0, 0, 0, 1), boardtest.Pair(t, 3, 1, 3, 0), boardtest.Pair(t, 1, 1, 2, 1))

	_, events := apply(t, f.changeLayout(t, f.register(t), before), f.changeLayoutCommand(after), changedAt)

	require.Equal(t, []board.Event{&board.LayoutChangedEvent{
		EventMeta: board.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Paired:    []board.Pair{boardtest.Pair(t, 0, 0, 0, 1), boardtest.Pair(t, 3, 0, 3, 1)},
		Unpaired:  []board.Pair{boardtest.Pair(t, 0, 0, 3, 0)},
		ChangedBy: f.operatorID,
	}}, events)
}

func TestDecide_ChangeLayoutEmitsEventWhenOnlyPairsChange(t *testing.T) {
	f := newFixture(t)
	size := boardtest.Size(t, 2, 1)
	tiles := []board.Tile{boardtest.Tile(t, 0, 0, "bridge", ""), boardtest.Tile(t, 1, 0, "bridge", "")}
	before := boardtest.Layout(t, size, tiles, nil)
	after := boardtest.Layout(t, size, tiles, nil, boardtest.Pair(t, 0, 0, 1, 0))

	state, events := apply(t, f.changeLayout(t, f.register(t), before), f.changeLayoutCommand(after), changedAt)

	require.Len(t, events, 1)
	require.Equal(t, []board.Pair{boardtest.Pair(t, 0, 0, 1, 0)}, events[0].(*board.LayoutChangedEvent).Paired)
	require.Equal(t, after, state.(board.Board).Layout)
}

// マスを消すと、そのマスの対も切れる（前と後の集合の差なので Unpaired に入る）。種類を変えて対にできなくなった場合も同じ
func TestDecide_ChangeLayoutUnpairsPairsOfRemovedOrChangedTile(t *testing.T) {
	f := newFixture(t)
	size := boardtest.Size(t, 3, 1)
	tiles := []board.Tile{boardtest.Tile(t, 0, 0, "warp", ""), boardtest.Tile(t, 1, 0, "warp", ""), boardtest.Tile(t, 2, 0, "bridge", "")}
	withPairs := f.changeLayout(t, f.register(t), boardtest.Layout(t, size, tiles, nil, boardtest.Pair(t, 0, 0, 1, 0)))
	removed := boardtest.Layout(t, size, tiles[:1], nil)
	changed := boardtest.Layout(t, size, []board.Tile{boardtest.Tile(t, 0, 0, "warp", ""), boardtest.Tile(t, 1, 0, "sanctuary", ""), tiles[2]}, nil)

	_, removedEvents := apply(t, withPairs, f.changeLayoutCommand(removed), changedAt)
	_, changedEvents := apply(t, withPairs, f.changeLayoutCommand(changed), changedAt)

	require.Equal(t, []board.Pair{boardtest.Pair(t, 0, 0, 1, 0)}, removedEvents[0].(*board.LayoutChangedEvent).Unpaired)
	require.Equal(t, []board.Pair{boardtest.Pair(t, 0, 0, 1, 0)}, changedEvents[0].(*board.LayoutChangedEvent).Unpaired)
}

func TestEvolve_RejectsPairThatBreaksLayoutRules(t *testing.T) {
	f := newFixture(t)
	event := &board.LayoutChangedEvent{
		EventMeta: board.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Paired:    []board.Pair{boardtest.Pair(t, 0, 0, 1, 0)},
		ChangedBy: f.operatorID,
	}

	_, err := board.Evolve(f.publishableDraft(t), event)

	require.ErrorIs(t, err, board.ErrInvalidLayout)
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
		RemovedTiles: []board.Position{boardtest.Position(t, 2, 0)},
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
		[]board.Connection{boardtest.Connection(t, 1, 1, 1, 0), boardtest.Connection(t, 1, 1, 0, 1), boardtest.Connection(t, 1, 1, 2, 1)})

	state, _ := apply(t, f.changeLayout(t, f.register(t), layout), f.publishCommand(), changedAt)

	require.Equal(t, board.StatusPublished, state.(board.Board).Status)
}

func TestDecide_RejectsPublishUnlessConditionsAreMet(t *testing.T) {
	f := newFixture(t)
	size := boardtest.Size(t, 3, 3)
	tests := []struct {
		name        string
		layout      board.Layout
		wantBases   []board.Position
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
			wantBases:  []board.Position{boardtest.Position(t, 0, 0), boardtest.Position(t, 1, 0)},
			wantChecks: 1,
		},
		{
			name:      "no_checkpoint",
			layout:    boardtest.Layout(t, size, []board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "land", "fire")}, []board.Connection{boardtest.Connection(t, 0, 0, 1, 0)}),
			wantBases: []board.Position{boardtest.Position(t, 0, 0)},
		},
		{
			name: "unreachable_tiles",
			layout: boardtest.Layout(t, size,
				[]board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", ""), boardtest.Tile(t, 2, 2, "land", "fire"), boardtest.Tile(t, 0, 2, "land", "fire"), boardtest.Tile(t, 1, 2, "checkpoint", "")},
				[]board.Connection{boardtest.Connection(t, 0, 0, 1, 0), boardtest.Connection(t, 0, 2, 1, 2)}),
			wantBases:   []board.Position{boardtest.Position(t, 0, 0)},
			wantChecks:  2,
			wantUnreach: []board.Position{boardtest.Position(t, 0, 2), boardtest.Position(t, 1, 2), boardtest.Position(t, 2, 2)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := board.Decide(f.changeLayout(t, f.register(t), tt.layout), f.publishCommand(), changedAt)

			require.ErrorIs(t, err, board.ErrNotPublishable)
			var notPublishable *board.NotPublishableError
			require.True(t, errors.As(err, &notPublishable))
			require.Equal(t, board.Readiness{BasePositions: tt.wantBases, CheckpointCount: tt.wantChecks, UnreachableTiles: tt.wantUnreach}, notPublishable.Readiness)
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
		{name: "change_default_rules", cmd: f.changeDefaultRulesCommand(boardtest.DefaultRules(t, 5000, 0, 200, 50, 12, 2, 8))},
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
		{name: "change_default_rules", cmd: f.changeDefaultRulesCommand(boardtest.DefaultRules(t, 5000, 0, 200, 50, 12, 2, 8))},
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

func TestNewTileKind_AcceptsSpecialTileKinds(t *testing.T) {
	for _, value := range []string{"warp", "bridge", "sanctuary", "shrine", "prophecy_shop", "card_shop"} {
		t.Run(value, func(t *testing.T) {
			kind, err := board.NewTileKind(value)

			require.NoError(t, err)
			require.Equal(t, value, kind.String())
		})
	}
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

func TestNewElement_RejectsUnknownValue(t *testing.T) {
	for _, value := range []string{"fire", "earth", "water", "wind", "neutral", "multi"} {
		element, err := board.NewElement(value)
		require.NoError(t, err)
		require.Equal(t, value, element.String())
	}

	_, err := board.NewElement("light")

	require.ErrorIs(t, err, board.ErrElementUnknown)
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

func TestNewDefaultRules_ChecksEachRule(t *testing.T) {
	tests := []struct {
		name                                                            string
		target, starting, lap, checkpoint, roundLimit, diceMin, diceMax int
		wantErrs                                                        []error
	}{
		{name: "minimum", target: 1, starting: 0, lap: 0, checkpoint: 0, roundLimit: 1, diceMin: 1, diceMax: 2},
		{name: "beyond_default_values", target: 1_000_000, starting: 999_999, lap: 1_000_000, checkpoint: 1_000_000, roundLimit: 1_000_000, diceMin: 1_000, diceMax: 1_000_000},
		{name: "zero_target", target: 0, starting: 0, lap: 0, checkpoint: 0, roundLimit: 1, diceMin: 1, diceMax: 6, wantErrs: []error{board.ErrTargetMagicTooSmall, board.ErrTargetMagicNotGreaterThanStartingMagic}},
		{name: "negative_target", target: -1, starting: 0, lap: 0, checkpoint: 0, roundLimit: 1, diceMin: 1, diceMax: 6, wantErrs: []error{board.ErrTargetMagicTooSmall, board.ErrTargetMagicNotGreaterThanStartingMagic}},
		{name: "negative_starting", target: 100, starting: -1, lap: 0, checkpoint: 0, roundLimit: 1, diceMin: 1, diceMax: 6, wantErrs: []error{board.ErrStartingMagicTooSmall}},
		{name: "negative_lap_bonus", target: 100, starting: 0, lap: -1, checkpoint: 0, roundLimit: 1, diceMin: 1, diceMax: 6, wantErrs: []error{board.ErrLapBonusTooSmall}},
		{name: "negative_checkpoint_bonus", target: 100, starting: 0, lap: 0, checkpoint: -1, roundLimit: 1, diceMin: 1, diceMax: 6, wantErrs: []error{board.ErrCheckpointBonusTooSmall}},
		{name: "target_equal_to_starting", target: 600, starting: 600, lap: 0, checkpoint: 0, roundLimit: 1, diceMin: 1, diceMax: 6, wantErrs: []error{board.ErrTargetMagicNotGreaterThanStartingMagic}},
		{name: "target_less_than_starting", target: 500, starting: 600, lap: 0, checkpoint: 0, roundLimit: 1, diceMin: 1, diceMax: 6, wantErrs: []error{board.ErrTargetMagicNotGreaterThanStartingMagic}},
		{name: "zero_round_limit", target: 100, starting: 0, lap: 0, checkpoint: 0, roundLimit: 0, diceMin: 1, diceMax: 6, wantErrs: []error{board.ErrRoundLimitTooSmall}},
		{name: "zero_dice_min", target: 100, starting: 0, lap: 0, checkpoint: 0, roundLimit: 1, diceMin: 0, diceMax: 6, wantErrs: []error{board.ErrDiceMinTooSmall}},
		{name: "dice_max_less_than_dice_min", target: 100, starting: 0, lap: 0, checkpoint: 0, roundLimit: 1, diceMin: 4, diceMax: 3, wantErrs: []error{board.ErrDiceMaxNotGreaterThanDiceMin}},
		{name: "dice_max_equal_to_dice_min", target: 100, starting: 0, lap: 0, checkpoint: 0, roundLimit: 1, diceMin: 3, diceMax: 3, wantErrs: []error{board.ErrDiceMaxNotGreaterThanDiceMin}},
		{name: "reports_all_violations", target: 0, starting: 0, lap: -1, checkpoint: -1, roundLimit: 0, diceMin: 0, diceMax: -1, wantErrs: []error{
			board.ErrTargetMagicTooSmall, board.ErrLapBonusTooSmall, board.ErrCheckpointBonusTooSmall, board.ErrTargetMagicNotGreaterThanStartingMagic,
			board.ErrRoundLimitTooSmall, board.ErrDiceMinTooSmall,
		}},
	}
	all := []error{
		board.ErrTargetMagicTooSmall, board.ErrStartingMagicTooSmall, board.ErrLapBonusTooSmall, board.ErrCheckpointBonusTooSmall,
		board.ErrTargetMagicNotGreaterThanStartingMagic, board.ErrRoundLimitTooSmall, board.ErrDiceMinTooSmall, board.ErrDiceMaxNotGreaterThanDiceMin,
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := board.NewDefaultRules(tt.target, tt.starting, tt.lap, tt.checkpoint, tt.roundLimit, tt.diceMin, tt.diceMax)

			if len(tt.wantErrs) == 0 {
				require.NoError(t, err)
				require.Equal(t, []int{tt.target, tt.starting, tt.lap, tt.checkpoint, tt.roundLimit, tt.diceMin, tt.diceMax},
					[]int{rules.TargetMagic, rules.StartingMagic, rules.LapBonus, rules.CheckpointBonus, rules.RoundLimit, rules.DiceMin, rules.DiceMax})
				return
			}
			got := slices.DeleteFunc(slices.Clone(all), func(e error) bool { return !errors.Is(err, e) })
			require.ElementsMatch(t, tt.wantErrs, got)
		})
	}
}

// 登録の直後の値が、保存のときの決まりを満たさなければ、登録しただけで違反のボードができてしまう
func TestInitialDefaultRules_SatisfiesTheRules(t *testing.T) {
	rules := board.InitialDefaultRules()

	checked, err := board.NewDefaultRules(rules.TargetMagic, rules.StartingMagic, rules.LapBonus, rules.CheckpointBonus, rules.RoundLimit, rules.DiceMin, rules.DiceMax)

	require.NoError(t, err)
	require.Equal(t, rules, checked)
	require.Equal(t, initialDefaultRules(t), rules)
}

func TestNewXAndY_AcceptZeroOrMore(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{name: "zero", value: 0},
		{name: "positive", value: 31},
		{name: "negative", value: -1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, xErr := board.NewX(tt.value)
			y, yErr := board.NewY(tt.value)

			if tt.wantErr {
				require.ErrorIs(t, xErr, board.ErrPositionNegative)
				require.ErrorIs(t, yErr, board.ErrPositionNegative)
				return
			}
			require.NoError(t, xErr)
			require.NoError(t, yErr)
			require.Equal(t, tt.value, x.Int())
			require.Equal(t, tt.value, y.Int())
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
		pairs       []board.Pair
		want        []board.LayoutViolation
	}{
		{name: "valid", tiles: []board.Tile{land(0, 0), land(2, 1)}},
		{
			name:  "special_tiles_with_element",
			tiles: []board.Tile{{Position: boardtest.Position(t, 0, 0), Kind: boardtest.TileKind(t, "warp"), Element: boardtest.Element(t, "fire")}, {Position: boardtest.Position(t, 1, 0), Kind: boardtest.TileKind(t, "card_shop"), Element: boardtest.Element(t, "wind")}},
			want: []board.LayoutViolation{
				{Subject: board.SubjectTile, Index: 0, Part: board.PartElement, Reason: board.ReasonElementNotAllowed},
				{Subject: board.SubjectTile, Index: 1, Part: board.PartElement, Reason: board.ReasonElementNotAllowed},
			},
		},
		{
			name: "pairs_of_distant_warps_and_bridges",
			tiles: []board.Tile{
				boardtest.Tile(t, 0, 0, "warp", ""), boardtest.Tile(t, 2, 1, "warp", ""), boardtest.Tile(t, 1, 0, "bridge", ""), boardtest.Tile(t, 0, 1, "bridge", ""),
			},
			pairs: []board.Pair{boardtest.Pair(t, 2, 1, 0, 0), boardtest.Pair(t, 1, 0, 0, 1)},
		},
		{
			name:  "pair_end_a_without_tile",
			tiles: []board.Tile{boardtest.Tile(t, 1, 0, "warp", "")},
			pairs: []board.Pair{boardtest.Pair(t, 0, 0, 1, 0)},
			want:  []board.LayoutViolation{{Subject: board.SubjectPair, Index: 0, Part: board.PartA, Reason: board.ReasonTileNotFound}},
		},
		{
			name:  "pair_end_b_without_tile",
			tiles: []board.Tile{boardtest.Tile(t, 0, 0, "bridge", "")},
			pairs: []board.Pair{boardtest.Pair(t, 0, 0, 1, 0)},
			want:  []board.LayoutViolation{{Subject: board.SubjectPair, Index: 0, Part: board.PartB, Reason: board.ReasonTileNotFound}},
		},
		{
			name:  "pair_end_is_land",
			tiles: []board.Tile{boardtest.Tile(t, 0, 0, "warp", ""), land(1, 0)},
			pairs: []board.Pair{boardtest.Pair(t, 0, 0, 1, 0)},
			want:  []board.LayoutViolation{{Subject: board.SubjectPair, Index: 0, Part: board.PartB, Reason: board.ReasonNotPairableTile}},
		},
		{
			name:  "pair_ends_are_facilities",
			tiles: []board.Tile{boardtest.Tile(t, 0, 0, "sanctuary", ""), boardtest.Tile(t, 1, 0, "shrine", "")},
			pairs: []board.Pair{boardtest.Pair(t, 0, 0, 1, 0)},
			want: []board.LayoutViolation{
				{Subject: board.SubjectPair, Index: 0, Part: board.PartA, Reason: board.ReasonNotPairableTile},
				{Subject: board.SubjectPair, Index: 0, Part: board.PartB, Reason: board.ReasonNotPairableTile},
			},
		},
		{
			name:  "warp_paired_with_bridge",
			tiles: []board.Tile{boardtest.Tile(t, 0, 0, "warp", ""), boardtest.Tile(t, 1, 0, "bridge", "")},
			pairs: []board.Pair{boardtest.Pair(t, 1, 0, 0, 0)},
			want:  []board.LayoutViolation{{Subject: board.SubjectPair, Index: 0, Part: board.PartWhole, Reason: board.ReasonMixedPairKinds}},
		},
		{
			name:  "tile_paired_with_itself",
			tiles: []board.Tile{boardtest.Tile(t, 0, 0, "warp", "")},
			pairs: []board.Pair{boardtest.Pair(t, 0, 0, 0, 0)},
			want:  []board.LayoutViolation{{Subject: board.SubjectPair, Index: 0, Part: board.PartWhole, Reason: board.ReasonPairedWithSelf}},
		},
		{
			name:  "tile_in_two_pairs",
			tiles: []board.Tile{boardtest.Tile(t, 0, 0, "warp", ""), boardtest.Tile(t, 1, 0, "warp", ""), boardtest.Tile(t, 2, 0, "warp", "")},
			pairs: []board.Pair{boardtest.Pair(t, 0, 0, 1, 0), boardtest.Pair(t, 2, 0, 0, 0)},
			want:  []board.LayoutViolation{{Subject: board.SubjectPair, Index: 1, Part: board.PartA, Reason: board.ReasonTileAlreadyPaired}},
		},
		{
			name:  "duplicated_pair_in_reverse_order",
			tiles: []board.Tile{boardtest.Tile(t, 0, 0, "bridge", ""), boardtest.Tile(t, 2, 1, "bridge", "")},
			pairs: []board.Pair{boardtest.Pair(t, 0, 0, 2, 1), boardtest.Pair(t, 2, 1, 0, 0)},
			want:  []board.LayoutViolation{{Subject: board.SubjectPair, Index: 1, Part: board.PartWhole, Reason: board.ReasonDuplicatedPair}},
		},
		{
			name:  "land_without_element",
			tiles: []board.Tile{land(0, 0), {Position: boardtest.Position(t, 1, 0), Kind: boardtest.TileKind(t, "land")}},
			want:  []board.LayoutViolation{{Subject: board.SubjectTile, Index: 1, Part: board.PartElement, Reason: board.ReasonElementRequired}},
		},
		{
			name:  "base_with_element",
			tiles: []board.Tile{{Position: boardtest.Position(t, 0, 0), Kind: boardtest.TileKind(t, "base"), Element: boardtest.Element(t, "fire")}},
			want:  []board.LayoutViolation{{Subject: board.SubjectTile, Index: 0, Part: board.PartElement, Reason: board.ReasonElementNotAllowed}},
		},
		{
			name:  "checkpoint_with_element",
			tiles: []board.Tile{{Position: boardtest.Position(t, 0, 0), Kind: boardtest.TileKind(t, "checkpoint"), Element: boardtest.Element(t, "multi")}},
			want:  []board.LayoutViolation{{Subject: board.SubjectTile, Index: 0, Part: board.PartElement, Reason: board.ReasonElementNotAllowed}},
		},
		{
			name:  "out_of_range_land_without_element",
			tiles: []board.Tile{{Position: boardtest.Position(t, 3, 0), Kind: boardtest.TileKind(t, "land")}},
			want: []board.LayoutViolation{
				{Subject: board.SubjectTile, Index: 0, Part: board.PartX, Reason: board.ReasonOutOfRange},
				{Subject: board.SubjectTile, Index: 0, Part: board.PartElement, Reason: board.ReasonElementRequired},
			},
		},
		{
			name:  "x_out_of_range",
			tiles: []board.Tile{land(0, 0), land(3, 0)},
			want:  []board.LayoutViolation{{Subject: board.SubjectTile, Index: 1, Part: board.PartX, Reason: board.ReasonOutOfRange}},
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
			connections: []board.Connection{boardtest.Connection(t, 0, 0, 1, 0)},
			want:        []board.LayoutViolation{{Subject: board.SubjectConnection, Index: 0, Part: board.PartA, Reason: board.ReasonTileNotFound}},
		},
		{
			name:        "end_b_without_tile",
			tiles:       []board.Tile{land(0, 0)},
			connections: []board.Connection{boardtest.Connection(t, 0, 0, 1, 0)},
			want:        []board.LayoutViolation{{Subject: board.SubjectConnection, Index: 0, Part: board.PartB, Reason: board.ReasonTileNotFound}},
		},
		{
			name:        "ends_outside_the_grid_without_tiles",
			connections: []board.Connection{boardtest.Connection(t, 5, 5, 5, 6)},
			want: []board.LayoutViolation{
				{Subject: board.SubjectConnection, Index: 0, Part: board.PartA, Reason: board.ReasonTileNotFound},
				{Subject: board.SubjectConnection, Index: 0, Part: board.PartB, Reason: board.ReasonTileNotFound},
			},
		},
		{
			name:        "diagonal",
			tiles:       []board.Tile{land(0, 0), land(1, 1)},
			connections: []board.Connection{boardtest.Connection(t, 0, 0, 1, 1)},
			want:        []board.LayoutViolation{{Subject: board.SubjectConnection, Index: 0, Part: board.PartWhole, Reason: board.ReasonNotAdjacent}},
		},
		{
			name:        "two_apart",
			tiles:       []board.Tile{land(0, 0), land(2, 0)},
			connections: []board.Connection{boardtest.Connection(t, 0, 0, 2, 0)},
			want:        []board.LayoutViolation{{Subject: board.SubjectConnection, Index: 0, Part: board.PartWhole, Reason: board.ReasonNotAdjacent}},
		},
		{
			name:        "same_position_at_both_ends",
			tiles:       []board.Tile{land(0, 0)},
			connections: []board.Connection{boardtest.Connection(t, 0, 0, 0, 0)},
			want:        []board.LayoutViolation{{Subject: board.SubjectConnection, Index: 0, Part: board.PartWhole, Reason: board.ReasonNotAdjacent}},
		},
		{
			name:        "duplicated_connection_in_reverse_order",
			tiles:       []board.Tile{land(0, 0), land(1, 0)},
			connections: []board.Connection{boardtest.Connection(t, 0, 0, 1, 0), boardtest.Connection(t, 1, 0, 0, 0)},
			want:        []board.LayoutViolation{{Subject: board.SubjectConnection, Index: 1, Part: board.PartWhole, Reason: board.ReasonDuplicatedConnection}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, board.ValidateLayout(size, tt.tiles, tt.connections, tt.pairs))
		})
	}
}

func TestNewLayout_RejectsViolationsWithTheirList(t *testing.T) {
	size := boardtest.Size(t, 2, 2)

	_, err := board.NewLayout(size, []board.Tile{boardtest.Tile(t, 2, 0, "land", "fire")}, nil, nil)

	require.ErrorIs(t, err, board.ErrInvalidLayout)
	var invalid *board.InvalidLayoutError
	require.True(t, errors.As(err, &invalid))
	require.Equal(t, []board.LayoutViolation{{Subject: board.SubjectTile, Index: 0, Part: board.PartX, Reason: board.ReasonOutOfRange}}, invalid.Violations)
}

func TestLayout_ListsTilesByRowThenColumnAndConnectionsFromA(t *testing.T) {
	layout := boardtest.Layout(t, boardtest.Size(t, 3, 3),
		[]board.Tile{boardtest.Tile(t, 0, 1, "land", "fire"), boardtest.Tile(t, 2, 0, "land", "fire"), boardtest.Tile(t, 1, 0, "land", "fire"), boardtest.Tile(t, 1, 1, "land", "fire")},
		[]board.Connection{boardtest.Connection(t, 1, 1, 1, 0), boardtest.Connection(t, 1, 0, 2, 0), boardtest.Connection(t, 0, 1, 1, 1)})

	require.Equal(t, []board.Tile{
		boardtest.Tile(t, 1, 0, "land", "fire"), boardtest.Tile(t, 2, 0, "land", "fire"), boardtest.Tile(t, 0, 1, "land", "fire"), boardtest.Tile(t, 1, 1, "land", "fire"),
	}, layout.Tiles())
	require.Equal(t, []board.Connection{
		boardtest.Connection(t, 1, 0, 2, 0), boardtest.Connection(t, 1, 0, 1, 1), boardtest.Connection(t, 0, 1, 1, 1),
	}, layout.Connections())
}

func TestLayout_ListsPairsFromA(t *testing.T) {
	layout := boardtest.Layout(t, boardtest.Size(t, 3, 3),
		[]board.Tile{
			boardtest.Tile(t, 2, 2, "warp", ""), boardtest.Tile(t, 0, 0, "warp", ""), boardtest.Tile(t, 1, 0, "bridge", ""), boardtest.Tile(t, 0, 2, "bridge", ""),
		}, nil, boardtest.Pair(t, 0, 2, 1, 0), boardtest.Pair(t, 2, 2, 0, 0))

	require.Equal(t, []board.Pair{boardtest.Pair(t, 0, 0, 2, 2), boardtest.Pair(t, 1, 0, 0, 2)}, layout.Pairs())
}

func TestNewPair_PutsSmallerPositionFirst(t *testing.T) {
	pair := boardtest.Pair(t, 2, 1, 0, 0)

	require.Equal(t, boardtest.Position(t, 0, 0), pair.A)
	require.Equal(t, boardtest.Position(t, 2, 1), pair.B)
	require.Equal(t, pair, boardtest.Pair(t, 0, 0, 2, 1))
}

func TestNewConnection_PutsSmallerPositionFirst(t *testing.T) {
	connection := boardtest.Connection(t, 2, 1, 2, 0)

	require.Equal(t, boardtest.Position(t, 2, 0), connection.A)
	require.Equal(t, boardtest.Position(t, 2, 1), connection.B)
	require.Equal(t, connection, boardtest.Connection(t, 2, 0, 2, 1))
}

func TestReadiness_LeavesUnreachableEmptyUnlessExactlyOneBase(t *testing.T) {
	size := boardtest.Size(t, 3, 3)
	layout := boardtest.Layout(t, size,
		[]board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "base", ""), boardtest.Tile(t, 2, 2, "land", "fire")}, nil)

	require.Equal(t, board.Readiness{BasePositions: []board.Position{boardtest.Position(t, 0, 0), boardtest.Position(t, 1, 0)}}, layout.Readiness())
}

func TestReadiness_TracesConnectionsInEitherDirection(t *testing.T) {
	size := boardtest.Size(t, 3, 3)
	layout := boardtest.Layout(t, size,
		[]board.Tile{boardtest.Tile(t, 2, 2, "base", ""), boardtest.Tile(t, 1, 2, "land", "fire"), boardtest.Tile(t, 0, 2, "checkpoint", "")},
		[]board.Connection{boardtest.Connection(t, 0, 2, 1, 2), boardtest.Connection(t, 1, 2, 2, 2)})

	readiness := layout.Readiness()

	require.Equal(t, board.Readiness{BasePositions: []board.Position{boardtest.Position(t, 2, 2)}, CheckpointCount: 1}, readiness)
	require.True(t, readiness.Publishable())
}

func TestReadiness_CountsPairsAsRoadsInEitherDirection(t *testing.T) {
	size := boardtest.Size(t, 5, 1)
	// 拠点 → 転送円(1) と、離れた転送円(3) → 橋(4)... (3)(4) は対でだけ拠点側とつながる
	layout := boardtest.Layout(t, size,
		[]board.Tile{
			boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "warp", ""), boardtest.Tile(t, 2, 0, "checkpoint", ""), boardtest.Tile(t, 3, 0, "warp", ""),
		},
		[]board.Connection{boardtest.Connection(t, 0, 0, 1, 0)}, boardtest.Pair(t, 3, 0, 1, 0))

	readiness := layout.Readiness()

	// (2,0) は隣の (1,0)・(3,0) とつながっていないので、対で届く (3,0) だけが行ける
	require.Equal(t, []board.Position{boardtest.Position(t, 2, 0)}, readiness.UnreachableTiles)
	require.Empty(t, readiness.UnpairedTiles)
}

func TestReadiness_ListsUnpairedWarpsAndBridgesEvenWithoutExactlyOneBase(t *testing.T) {
	layout := boardtest.Layout(t, boardtest.Size(t, 4, 4),
		[]board.Tile{
			boardtest.Tile(t, 3, 1, "warp", ""), boardtest.Tile(t, 0, 2, "bridge", ""), boardtest.Tile(t, 1, 2, "bridge", ""), boardtest.Tile(t, 2, 3, "sanctuary", ""),
			boardtest.Tile(t, 0, 0, "warp", ""), boardtest.Tile(t, 1, 0, "warp", ""),
		}, nil, boardtest.Pair(t, 0, 2, 1, 2), boardtest.Pair(t, 0, 0, 1, 0))

	require.Equal(t, []board.Position{boardtest.Position(t, 3, 1)}, layout.Readiness().UnpairedTiles)
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
			EventMeta: meta, PublicID: publicID, Name: f.name, Width: boardtest.Width(t, 8), Height: boardtest.Height(t, 8), DefaultRules: initialDefaultRules(t), RegisteredBy: f.operatorID,
		},
		&board.RenamedEvent{EventMeta: meta, Name: boardtest.Name(t, "砂漠"), RenamedBy: f.operatorID},
		&board.LayoutChangedEvent{
			EventMeta:    meta,
			Size:         &size,
			PlacedTiles:  []board.Tile{boardtest.Tile(t, 1, 0, "land", "fire"), boardtest.Tile(t, 0, 1, "base", ""), boardtest.Tile(t, 0, 0, "warp", ""), boardtest.Tile(t, 3, 0, "warp", "")},
			RemovedTiles: []board.Position{boardtest.Position(t, 2, 1)},
			ChangedTiles: []board.Tile{boardtest.Tile(t, 3, 2, "checkpoint", ""), boardtest.Tile(t, 3, 1, "land", "multi")},
			Connected:    []board.Connection{boardtest.Connection(t, 0, 1, 1, 1)},
			Disconnected: []board.Connection{boardtest.Connection(t, 1, 1, 2, 1)},
			Paired:       []board.Pair{boardtest.Pair(t, 0, 0, 3, 0)},
			Unpaired:     []board.Pair{boardtest.Pair(t, 1, 2, 2, 2)},
			ChangedBy:    f.operatorID,
		},
		&board.DefaultRulesChangedEvent{EventMeta: meta, DefaultRules: boardtest.DefaultRules(t, 5000, 0, 200, 50, 12, 2, 8), ChangedBy: f.operatorID},
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

	require.JSONEq(t, `{"placed_tiles":[{"position":{"x":1,"y":0},"kind":"land","element":"fire"}],"changed_by":10}`, string(payload))
	restored, err := board.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), 1, payload)
	require.NoError(t, err)
	require.Equal(t, event, restored)
}

// 対を足す前に保存したイベント（paired・unpaired が無い payload）を、対の増減が無いものとして読める
func TestEvent_LayoutChangedReadsPayloadWithoutPairs(t *testing.T) {
	f := newFixture(t)
	meta := board.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt}
	payload := []byte(`{"connected":[{"a":{"x":0,"y":1},"b":{"x":1,"y":1}}],"changed_by":10}`)

	restored, err := board.NewEventType(domain.NewEventName("LayoutChangedEvent")).Unmarshal(meta, 1, payload)

	require.NoError(t, err)
	require.Equal(t, &board.LayoutChangedEvent{
		EventMeta: meta, Connected: []board.Connection{boardtest.Connection(t, 0, 1, 1, 1)}, ChangedBy: f.operatorID,
	}, restored)
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := board.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(board.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}

func TestReadiness_ListsProblemsInFixedOrderWithPositions(t *testing.T) {
	size := boardtest.Size(t, 4, 4)
	tests := []struct {
		name   string
		layout board.Layout
		want   []board.PublishProblem
	}{
		{name: "empty", layout: board.EmptyLayout(size), want: []board.PublishProblem{
			{Kind: board.PublishProblemBaseCount},
			{Kind: board.PublishProblemNoCheckpoint},
		}},
		{
			name: "two_bases_without_checkpoint",
			layout: boardtest.Layout(t, size,
				[]board.Tile{boardtest.Tile(t, 3, 1, "base", ""), boardtest.Tile(t, 0, 2, "base", "")}, nil),
			want: []board.PublishProblem{
				{Kind: board.PublishProblemBaseCount, Positions: []board.Position{boardtest.Position(t, 3, 1), boardtest.Position(t, 0, 2)}},
				{Kind: board.PublishProblemNoCheckpoint},
			},
		},
		{
			name: "unreachable_after_exactly_one_base",
			layout: boardtest.Layout(t, size,
				[]board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", ""), boardtest.Tile(t, 3, 3, "land", "fire")},
				[]board.Connection{boardtest.Connection(t, 0, 0, 1, 0)}),
			want: []board.PublishProblem{
				{Kind: board.PublishProblemUnreachableTiles, Positions: []board.Position{boardtest.Position(t, 3, 3)}},
			},
		},
		{
			name: "unpaired_after_unreachable",
			layout: boardtest.Layout(t, size,
				[]board.Tile{
					boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", ""), boardtest.Tile(t, 3, 3, "land", "fire"),
					boardtest.Tile(t, 1, 1, "warp", ""), boardtest.Tile(t, 0, 1, "bridge", ""),
				},
				[]board.Connection{boardtest.Connection(t, 0, 0, 1, 0), boardtest.Connection(t, 1, 0, 1, 1), boardtest.Connection(t, 0, 0, 0, 1)}),
			want: []board.PublishProblem{
				{Kind: board.PublishProblemUnreachableTiles, Positions: []board.Position{boardtest.Position(t, 3, 3)}},
				{Kind: board.PublishProblemUnpairedTiles, Positions: []board.Position{boardtest.Position(t, 0, 1), boardtest.Position(t, 1, 1)}},
			},
		},
		{
			name: "unpaired_without_base_comes_last",
			layout: boardtest.Layout(t, size,
				[]board.Tile{boardtest.Tile(t, 2, 2, "bridge", "")}, nil),
			want: []board.PublishProblem{
				{Kind: board.PublishProblemBaseCount},
				{Kind: board.PublishProblemNoCheckpoint},
				{Kind: board.PublishProblemUnpairedTiles, Positions: []board.Position{boardtest.Position(t, 2, 2)}},
			},
		},
		{
			name: "pairs_and_facilities_publishable",
			layout: boardtest.Layout(t, size,
				[]board.Tile{
					boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", ""), boardtest.Tile(t, 2, 0, "warp", ""), boardtest.Tile(t, 3, 3, "warp", ""),
					boardtest.Tile(t, 3, 0, "shrine", ""),
				},
				[]board.Connection{boardtest.Connection(t, 0, 0, 1, 0), boardtest.Connection(t, 1, 0, 2, 0), boardtest.Connection(t, 2, 0, 3, 0)}, boardtest.Pair(t, 2, 0, 3, 3)),
		},
		{
			name: "publishable",
			layout: boardtest.Layout(t, size,
				[]board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", "")},
				[]board.Connection{boardtest.Connection(t, 0, 0, 1, 0)}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.layout.Readiness().Problems())
		})
	}
}
