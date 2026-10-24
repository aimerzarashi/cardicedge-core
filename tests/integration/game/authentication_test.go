package game_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
)

const versionQuery = `{ version }`

func gameClaims(subject string) idptest.Claims {
	claims := idptest.ValidClaims(subject)
	claims.Issuer = idptest.GameIssuer
	return claims
}

func TestAuthentication_AcceptsTokenIssuedByGame(t *testing.T) {
	t.Parallel()

	status, body := client.Post(t, key.Sign(t, gameClaims("player-1")), versionQuery, nil)

	require.Equal(t, http.StatusOK, status)
	require.Nil(t, body["errors"])
	require.Equal(t, "test", body["data"].(map[string]any)["version"])
}

// studio の JWT は、同じ鍵・同じ aud でも、発行元（iss）が違うため通さない
func TestAuthentication_RejectsRequestWithoutValidToken(t *testing.T) {
	t.Parallel()
	expired := gameClaims("player-1")
	expired.ExpiresAt = expired.IssuedAt
	tests := []struct {
		name  string
		token string
	}{
		{name: "without_token", token: ""},
		{name: "malformed_token", token: "not-a-jwt"},
		{name: "signed_by_other_key", token: idptest.NewKey(t, "k1").Sign(t, gameClaims("player-1"))},
		{name: "issued_by_studio", token: key.Sign(t, idptest.ValidClaims("player-1"))},
		{name: "expired", token: key.Sign(t, expired)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := client.Post(t, tt.token, versionQuery, nil)

			require.Equal(t, http.StatusUnauthorized, status)
			require.Equal(t, "UNAUTHENTICATED", body["errors"].([]any)[0].(map[string]any)["extensions"].(map[string]any)["code"])
		})
	}
}

func TestAuthentication_RejectsGetRequest(t *testing.T) {
	t.Parallel()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint+"?query=%7Bversion%7D", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key.Sign(t, gameClaims("player-1")))

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.NotEqual(t, http.StatusOK, res.StatusCode)
}
