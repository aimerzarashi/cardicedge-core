package studiorulestest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 生成関数のエラーを毎回確かめると冗長なため、ここで確かめる

func ID(t testing.TB, value int64) studiorules.ID {
	t.Helper()
	id, err := studiorules.NewID(value)
	require.NoError(t, err)
	return id
}

// ボードの幅・高さの範囲。値の形（下限が 1 以上、上限が下限以上）を満たさない範囲はテストに使えない
func Board(t testing.TB, minSize, maxSize int) board.SizeRange {
	t.Helper()
	return boardtest.SizeRange(t, minSize, maxSize)
}

func Rules(t testing.TB, minSize, maxSize int) studiorules.Rules {
	t.Helper()
	return studiorules.Rules{Board: Board(t, minSize, maxSize)}
}
