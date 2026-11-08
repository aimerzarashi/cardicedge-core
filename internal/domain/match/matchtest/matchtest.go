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
		gamerulestest.RewardCardsByRank(t, 3, 2, 1, 1),
		match.DefaultRewardPacks(),
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

func (Random) IntN(int) int {
	return 0
}

// 決めた目を順に返す乱数の元（目の最小が 1 のルール用）。Shuffle は Random と同じ逆順
type RollingRandom struct {
	Random
	faces []int
}

func NewRollingRandom(faces ...int) *RollingRandom {
	return &RollingRandom{faces: faces}
}

func (r *RollingRandom) IntN(int) int {
	if len(r.faces) == 0 {
		panic("matchtest: no dice faces left")
	}
	face := r.faces[0]
	r.faces = r.faces[1:]
	return face - 1
}

// 最大の目を返す乱数の元
type HighestRandom struct {
	Random
}

func NewHighestRandom() HighestRandom {
	return HighestRandom{}
}

func (HighestRandom) IntN(n int) int {
	return n - 1
}

// すべての値を指定するルール（ボードの既定の値は board.DefaultRules で受け取る）
func RulesOf(t testing.TB, targetMagic, startingMagic, lapBonus, checkpointBonus, roundLimit, diceMin, diceMax, startingHandCards int) match.Rules {
	t.Helper()
	return match.NewRules(
		boardtest.DefaultRules(t, targetMagic, startingMagic, lapBonus, checkpointBonus, roundLimit, diceMin, diceMax),
		gamerulestest.MaxHandCards(t, 6),
		gamerulestest.StartingHandCards(t, startingHandCards),
		gamerulestest.RewardCardsByRank(t, 3, 2, 1, 1),
		match.DefaultRewardPacks(),
	)
}

func DeployedCreatureID(t testing.TB, value int) match.DeployedCreatureID {
	t.Helper()
	id, err := match.NewDeployedCreatureID(value)
	require.NoError(t, err)
	return id
}
