package studio_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

const gameRulesQuery = `query {
	gameRules {
		__typename
		... on GameRules { match { minPlayers maxPlayers maxHandCards } }
	}
}`

func TestGameRules_ReturnsMatchRulesToOperator(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)

	var rules map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		rules, err = client.TryData(t.Context(), token, gameRulesQuery, nil, "gameRules")
		if assert.NoError(c, err) {
			assert.Equal(c, "GameRules", rules["__typename"])
		}
	})

	defaults := gamerules.Defaults().Match
	require.Equal(t, map[string]any{
		"__typename": "GameRules",
		"match": map[string]any{
			"minPlayers":   float64(defaults.MinPlayers),
			"maxPlayers":   float64(defaults.MaxPlayers),
			"maxHandCards": float64(defaults.MaxHandCards),
		},
	}, rules)
}

func TestGameRules_IsForbiddenForNonOperator(t *testing.T) {
	t.Parallel()
	notOperator := key.Sign(t, idptest.ValidClaims("not-operator-"+testhelper.UniqueID(t)))

	status, body := client.Post(t, notOperator, gameRulesQuery, nil)

	require.Equal(t, http.StatusOK, status)
	require.Nil(t, body["errors"])
	require.Equal(t, "ForbiddenError", body["data"].(map[string]any)["gameRules"].(map[string]any)["__typename"])
}
