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

// 決まった目を順に返し、山札は逆順に並べる乱数の元。目が足りなくなったら、テストの用意の誤りとして落とす
type Random struct {
	t       testing.TB
	diceMin int
	values  []int
}

func NewRandom(t testing.TB, rules match.Rules, values ...int) *Random {
	t.Helper()
	return &Random{t: t, diceMin: rules.DiceMin, values: values}
}

func (r *Random) IntN(n int) int {
	r.t.Helper()
	require.NotEmpty(r.t, r.values)
	value := r.values[0] - r.diceMin
	r.values = r.values[1:]
	require.True(r.t, value >= 0 && value < n)
	return value
}

func (r *Random) Shuffle(n int, swap func(i, j int)) {
	for i := range n / 2 {
		swap(i, n-1-i)
	}
}

// 振っていない目が残っていない（振る回数がテストの想定どおり）
func (r *Random) RequireUsed(t testing.TB) {
	t.Helper()
	require.Empty(t, r.values)
}

func Index(t testing.TB, value int) match.ParticipantIndex {
	t.Helper()
	index, err := match.NewParticipantIndex(value)
	require.NoError(t, err)
	return index
}

func Roll(t testing.TB, participant, attempt, value int) match.TurnOrderRoll {
	t.Helper()
	return match.TurnOrderRoll{Participant: Index(t, participant), Attempt: attempt, Value: value}
}
