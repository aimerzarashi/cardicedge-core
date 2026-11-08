package gamerulestest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 生成関数のエラーを毎回確かめると冗長なため、ここで確かめる

func ID(t testing.TB, value int64) gamerules.ID {
	t.Helper()
	id, err := gamerules.NewID(value)
	require.NoError(t, err)
	return id
}

func Match(t testing.TB, minPlayers, maxPlayers, maxHandCards, startingHandCards int) gamerules.Match {
	t.Helper()
	min, err := gamerules.NewMinPlayers(minPlayers)
	require.NoError(t, err)
	max, err := gamerules.NewMaxPlayers(maxPlayers)
	require.NoError(t, err)
	hand, err := gamerules.NewMaxHandCards(maxHandCards)
	require.NoError(t, err)
	starting, err := gamerules.NewStartingHandCards(startingHandCards)
	require.NoError(t, err)
	return gamerules.Match{MinPlayers: min, MaxPlayers: max, MaxHandCards: hand, StartingHandCards: starting, RewardCardsByRank: RewardCardsByRank(t, 3, 2, 1, 1),
		LandLevelMultipliers: gamerules.Defaults().Match.LandLevelMultipliers, ChainMultipliers: gamerules.Defaults().Match.ChainMultipliers,
		TollPercentage: gamerules.Defaults().Match.TollPercentage, RelinquishPercentage: gamerules.Defaults().Match.RelinquishPercentage}
}

func ChainMultipliers(t testing.TB, values ...int) gamerules.ChainMultipliers {
	t.Helper()
	multipliers, err := gamerules.NewChainMultipliers(values...)
	require.NoError(t, err)
	return multipliers
}

func RewardCardsByRank(t testing.TB, values ...int) gamerules.RewardCardsByRank {
	t.Helper()
	counts, err := gamerules.NewRewardCardsByRank(values...)
	require.NoError(t, err)
	return counts
}

func MaxHandCards(t testing.TB, value int) gamerules.MaxHandCards {
	t.Helper()
	hand, err := gamerules.NewMaxHandCards(value)
	require.NoError(t, err)
	return hand
}

func StartingHandCards(t testing.TB, value int) gamerules.StartingHandCards {
	t.Helper()
	starting, err := gamerules.NewStartingHandCards(value)
	require.NoError(t, err)
	return starting
}
