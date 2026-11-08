package boardtest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 生成関数のエラーを毎回確かめると冗長なため、ここで確かめる

func ID(t testing.TB, value int64) board.ID {
	t.Helper()
	id, err := board.NewID(value)
	require.NoError(t, err)
	return id
}

func Name(t testing.TB, value string) board.Name {
	t.Helper()
	name, err := board.NewName(value)
	require.NoError(t, err)
	return name
}

// 幅・高さの範囲。値の形（下限が 1 以上、上限が下限以上）を満たさない範囲はテストに使えない
func SizeRange(t testing.TB, minSize, maxSize int) board.SizeRange {
	t.Helper()
	min, err := board.NewMinSize(minSize)
	require.NoError(t, err)
	max, err := board.NewMaxSize(maxSize)
	require.NoError(t, err)
	sizeRange, err := board.NewSizeRange(min, max)
	require.NoError(t, err)
	return sizeRange
}

// 範囲を気にしないテストが幅・高さを作るための、十分広い範囲。範囲の外を確かめるテストは SizeRange で範囲を作って生成関数を直接呼ぶ
func wideRange(t testing.TB) board.SizeRange {
	t.Helper()
	return SizeRange(t, 1, 1000)
}

func Width(t testing.TB, value int) board.Width {
	t.Helper()
	width, err := board.NewWidth(value, wideRange(t))
	require.NoError(t, err)
	return width
}

func Height(t testing.TB, value int) board.Height {
	t.Helper()
	height, err := board.NewHeight(value, wideRange(t))
	require.NoError(t, err)
	return height
}

func X(t testing.TB, value int) board.X {
	t.Helper()
	x, err := board.NewX(value)
	require.NoError(t, err)
	return x
}

func Y(t testing.TB, value int) board.Y {
	t.Helper()
	y, err := board.NewY(value)
	require.NoError(t, err)
	return y
}

func Size(t testing.TB, width, height int) board.Size {
	t.Helper()
	return board.Size{Width: Width(t, width), Height: Height(t, height)}
}

func DefaultRules(t testing.TB, targetMagic, startingMagic, lapBonus, checkpointBonus, roundLimit, diceMin, diceMax int) board.DefaultRules {
	t.Helper()
	rules, err := board.NewDefaultRules(targetMagic, startingMagic, lapBonus, checkpointBonus, roundLimit, diceMin, diceMax)
	require.NoError(t, err)
	return rules
}

func TileKind(t testing.TB, value string) board.TileKind {
	t.Helper()
	kind, err := board.NewTileKind(value)
	require.NoError(t, err)
	return kind
}

func Element(t testing.TB, value string) board.Element {
	t.Helper()
	element, err := board.NewElement(value)
	require.NoError(t, err)
	return element
}

// 属性は土地のときだけ渡す（拠点・チェックポイントは空にする）
func Tile(t testing.TB, x, y int, kind, element string) board.Tile {
	t.Helper()
	tile := board.Tile{Position: Position(t, x, y), Kind: TileKind(t, kind)}
	if element != "" {
		tile.Element = Element(t, element)
	}
	return tile
}

func Connection(t testing.TB, ax, ay, bx, by int) board.Connection {
	t.Helper()
	return board.NewConnection(Position(t, ax, ay), Position(t, bx, by))
}

func Pair(t testing.TB, ax, ay, bx, by int) board.Pair {
	t.Helper()
	return board.NewPair(Position(t, ax, ay), Position(t, bx, by))
}

// 対が要らないテストは pairs を渡さない
func Layout(t testing.TB, size board.Size, tiles []board.Tile, connections []board.Connection, pairs ...board.Pair) board.Layout {
	t.Helper()
	layout, err := board.NewLayout(size, tiles, connections, pairs)
	require.NoError(t, err)
	return layout
}

func Position(t testing.TB, x, y int) board.Position {
	t.Helper()
	return board.Position{X: X(t, x), Y: Y(t, y)}
}

func MaxSize(t testing.TB, value int) board.MaxSize {
	t.Helper()
	max, err := board.NewMaxSize(value)
	require.NoError(t, err)
	return max
}
