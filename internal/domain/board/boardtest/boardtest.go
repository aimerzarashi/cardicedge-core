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

func Size(t testing.TB, width, height int) board.Size {
	t.Helper()
	size, err := board.NewSize(width, height)
	require.NoError(t, err)
	return size
}

func DefaultRules(t testing.TB, targetMagic, startingMagic, lapBonus, checkpointBonus int) board.DefaultRules {
	t.Helper()
	rules, err := board.NewDefaultRules(targetMagic, startingMagic, lapBonus, checkpointBonus)
	require.NoError(t, err)
	return rules
}

func TileKind(t testing.TB, value string) board.TileKind {
	t.Helper()
	kind, err := board.NewTileKind(value)
	require.NoError(t, err)
	return kind
}

func Attribute(t testing.TB, value string) board.Attribute {
	t.Helper()
	attribute, err := board.NewAttribute(value)
	require.NoError(t, err)
	return attribute
}

// 属性は土地のときだけ渡す（拠点・チェックポイントは空にする）
func Tile(t testing.TB, x, y int, kind, attribute string) board.Tile {
	t.Helper()
	tile := board.Tile{Position: board.NewPosition(x, y), Kind: TileKind(t, kind)}
	if attribute != "" {
		tile.Attribute = Attribute(t, attribute)
	}
	return tile
}

func Connection(ax, ay, bx, by int) board.Connection {
	return board.NewConnection(board.NewPosition(ax, ay), board.NewPosition(bx, by))
}

func Layout(t testing.TB, size board.Size, tiles []board.Tile, connections []board.Connection) board.Layout {
	t.Helper()
	layout, err := board.NewLayout(size, tiles, connections)
	require.NoError(t, err)
	return layout
}

func Position(x, y int) board.Position {
	return board.NewPosition(x, y)
}
