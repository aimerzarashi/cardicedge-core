package studio_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

func TestAuthentication_LinksOperatorByVerifiedEmailOnFirstRequest(t *testing.T) {
	t.Parallel()
	id := testhelper.UniqueID(t)
	email, subject := "operator-"+id+"@example.com", "operator-"+id
	registered := operators.Register(t, email)

	status, body := client.Post(t, key.Sign(t, idptest.VerifiedClaims(subject, email)), starterDeckSetQuery, nil)

	require.Equal(t, http.StatusOK, status)
	require.Nil(t, body["errors"])
	linked, found := operators.Linked(t, subject)
	require.True(t, found)
	require.Equal(t, registered, linked)
}

// オペレーターかどうか・権限は操作ごとに認可で判定し、結果の型で返すため、入口では拒否しない
func TestAuthentication_AcceptsValidTokenOfNonOperator(t *testing.T) {
	t.Parallel()
	id := testhelper.UniqueID(t)
	email, subject := "operator-"+id+"@example.com", "operator-"+id
	operators.Register(t, email)
	unverified := idptest.ValidClaims(subject)
	unverified.Email = email
	tests := []struct {
		name   string
		claims idptest.Claims
	}{
		{name: "without_email", claims: idptest.ValidClaims(subject)},
		{name: "unverified_email", claims: unverified},
		{name: "unregistered_email", claims: idptest.VerifiedClaims(subject, "unregistered-"+email)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := client.Post(t, key.Sign(t, tt.claims), starterDeckSetQuery, nil)

			require.Equal(t, http.StatusOK, status)
			require.Equal(t, "ForbiddenError", body["data"].(map[string]any)["starterDeckSet"].(map[string]any)["__typename"])
			_, found := operators.Linked(t, subject)
			require.False(t, found)
		})
	}
}

func TestAuthentication_RejectsRequestWithoutValidToken(t *testing.T) {
	t.Parallel()
	otherIssuer := idptest.ValidClaims("user-1")
	otherIssuer.Issuer = idptest.GameIssuer
	tests := []struct {
		name  string
		token string
	}{
		{name: "without_token", token: ""},
		{name: "malformed_token", token: "not-a-jwt"},
		{name: "signed_by_other_key", token: idptest.NewKey(t, "k1").Sign(t, idptest.ValidClaims("user-1"))},
		{name: "issued_by_other_issuer", token: key.Sign(t, otherIssuer)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := client.Post(t, tt.token, starterDeckSetQuery, nil)

			require.Equal(t, http.StatusUnauthorized, status)
			require.Equal(t, "UNAUTHENTICATED", body["errors"].([]any)[0].(map[string]any)["extensions"].(map[string]any)["code"])
		})
	}
}

func TestAuthentication_RejectsGetRequest(t *testing.T) {
	t.Parallel()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint+"?query=%7BstarterDeckSet%7B__typename%7D%7D", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key.Sign(t, idptest.ValidClaims("user-1")))

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.NotEqual(t, http.StatusOK, res.StatusCode)
}
