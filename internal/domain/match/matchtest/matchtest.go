package matchtest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules/gamerulestest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
)

// テストでも値の型は生成関数で作る（factory_enforcer）

func ID(t testing.TB, value int64) match.ID {
	t.Helper()
	id, err := match.NewID(value)
	require.NoError(t, err)
	return id
}

// 目の範囲と配る枚数だけを変えたルール（ほかはボードの既定の値）
func Rules(t testing.TB, diceMin, diceMax, startingHandCards int) match.Rules {
	t.Helper()
	return match.NewRules(
		boardtest.DefaultRules(t, 8000, 600, 300, 100, 30, diceMin, diceMax),
		gamerulestest.MaxHandCards(t, 6),
		gamerulestest.StartingHandCards(t, startingHandCards),
	)
}

// 混ぜる並びを逆順にする乱数の元（順番も山札も、席の並び・デッキの並びの逆になる）
type Random struct{}

func NewRandom() Random {
	return Random{}
}

func (Random) Shuffle(n int, swap func(i, j int)) {
	for i := range n / 2 {
		swap(i, n-1-i)
	}
}

func Index(t testing.TB, value int) match.ParticipantIndex {
	t.Helper()
	index, err := match.NewParticipantIndex(value)
	require.NoError(t, err)
	return index
}
