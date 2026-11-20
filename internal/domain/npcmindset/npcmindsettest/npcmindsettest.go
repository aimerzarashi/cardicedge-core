package npcmindsettest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/matchaction"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 生成関数のエラーを毎回確かめると冗長なため、ここで確かめる

func ID(t testing.TB, value int64) npcmindset.ID {
	t.Helper()
	id, err := npcmindset.NewID(value)
	require.NoError(t, err)
	return id
}

func Name(t testing.TB, value string) npcmindset.Name {
	t.Helper()
	name, err := npcmindset.NewName(value)
	require.NoError(t, err)
	return name
}

func Weight(t testing.TB, value int) npcmindset.Weight {
	t.Helper()
	weight, err := npcmindset.NewWeight(value)
	require.NoError(t, err)
	return weight
}

// 値の範囲の確かめを通した重み。種類のすべてがそろっているかは確かめない（足りない・余分な入力のテストにも使うため）
func ActionWeights(t testing.TB, values map[matchaction.Action]int) npcmindset.ActionWeights {
	t.Helper()
	weights := npcmindset.ActionWeights{}
	for action, value := range values {
		weights[action] = Weight(t, value)
	}
	return weights
}

func CriterionWeights(t testing.TB, values map[npcmindset.Criterion]int) npcmindset.CriterionWeights {
	t.Helper()
	weights := npcmindset.CriterionWeights{}
	for criterion, value := range values {
		weights[criterion] = Weight(t, value)
	}
	return weights
}
