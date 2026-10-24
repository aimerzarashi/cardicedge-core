package game_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

const registerPlayerMutation = `mutation {
	registerPlayer {
		__typename
		... on RegisterPlayerSuccess { player { id } }
	}
}`

const meQuery = `{
	me {
		__typename
		... on Player { id }
		... on PlayerNotRegisteredError { message }
	}
}`

func gameClaims(subject string) idptest.Claims {
	claims := idptest.ValidClaims(subject)
	claims.Issuer = idptest.GameIssuer
	return claims
}

// 並列のテストが同じ DB を使うため、プレイヤーごとに別の sub の JWT を作る
func newPlayerToken(t *testing.T) string {
	t.Helper()
	return key.Sign(t, gameClaims("player-"+testhelper.UniqueID(t)))
}

func registerPlayer(t *testing.T, token string) map[string]any {
	t.Helper()
	return client.Data(t, token, registerPlayerMutation, nil, "registerPlayer")
}

func me(t *testing.T, token string) map[string]any {
	t.Helper()
	return client.Data(t, token, meQuery, nil, "me")
}

func registeredPlayerID(t *testing.T, result map[string]any) string {
	t.Helper()
	require.Equal(t, "RegisterPlayerSuccess", result["__typename"])
	return result["player"].(map[string]any)["id"].(string)
}
