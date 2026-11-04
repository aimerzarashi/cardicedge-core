package game_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
)

const gameRulesQuery = `query {
	gameRules {
		__typename
		... on GameRules { match { minPlayers maxPlayers maxHandCards startingHandCards } }
		... on PlayerNotRegisteredError { message }
	}
}`

func TestGameRules_ReturnsPublishedMatchRules(t *testing.T) {
	t.Parallel()

	result := client.Data(t, registeredPlayerToken(t), gameRulesQuery, nil, "gameRules")

	defaults := gamerules.Defaults().Match
	require.Equal(t, map[string]any{
		"__typename": "GameRules",
		"match": map[string]any{
			"minPlayers":        float64(defaults.MinPlayers),
			"maxPlayers":        float64(defaults.MaxPlayers),
			"maxHandCards":      float64(defaults.MaxHandCards),
			"startingHandCards": float64(defaults.StartingHandCards),
		},
	}, result)
}

func TestGameRules_ReturnsPlayerNotRegisteredForUnregisteredPlayer(t *testing.T) {
	t.Parallel()

	result := client.Data(t, newPlayerToken(t), gameRulesQuery, nil, "gameRules")

	require.Equal(t, "PlayerNotRegisteredError", result["__typename"])
	require.NotEmpty(t, result["message"])
}
