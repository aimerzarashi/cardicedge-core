package idp_test

import (
	"crypto/ed25519"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
)

func newVerifier(t *testing.T, keys ...idptest.Key) (*idp.Verifier, *idptest.JWKSServer) {
	t.Helper()
	server := idptest.NewJWKSServer(t, keys...)
	verifier, err := idp.NewVerifier(t.Context(), server.URL, idptest.Issuer, idptest.Audience)
	require.NoError(t, err)
	return verifier, server
}

func TestVerify_ReturnsClaimsOfValidToken(t *testing.T) {
	key := idptest.NewKey(t, "k1")
	verifier, _ := newVerifier(t, key)
	tests := []struct {
		name              string
		claims            idptest.Claims
		wantEmail         string
		wantEmailVerified bool
	}{
		{name: "without_email", claims: idptest.ValidClaims("user-1")},
		{
			name:      "with_verified_email",
			claims:    idptest.VerifiedClaims("user-1", "operator@example.com"),
			wantEmail: "operator@example.com", wantEmailVerified: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := verifier.Verify(key.Sign(t, tt.claims))

			require.NoError(t, err)
			require.Equal(t, "user-1", claims.Subject)
			require.Equal(t, tt.wantEmail, claims.Email)
			require.Equal(t, tt.wantEmailVerified, claims.EmailVerified)
		})
	}
}

func TestVerify_RejectsInvalidClaims(t *testing.T) {
	key := idptest.NewKey(t, "k1")
	verifier, _ := newVerifier(t, key)
	tests := []struct {
		name   string
		modify func(*idptest.Claims)
	}{
		{name: "other_issuer", modify: func(c *idptest.Claims) { c.Issuer = "diecardomain-game" }},
		{name: "other_audience", modify: func(c *idptest.Claims) { c.Audience = jwt.ClaimStrings{"other"} }},
		{name: "expired", modify: func(c *idptest.Claims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute)) }},
		{name: "without_expiration", modify: func(c *idptest.Claims) { c.ExpiresAt = nil }},
		{name: "empty_subject", modify: func(c *idptest.Claims) { c.Subject = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := idptest.ValidClaims("user-1")
			tt.modify(&claims)

			_, err := verifier.Verify(key.Sign(t, claims))

			require.ErrorIs(t, err, idp.ErrInvalidToken)
		})
	}
}

func TestVerify_RejectsTokenSignedByOtherKeyWithSameKid(t *testing.T) {
	verifier, _ := newVerifier(t, idptest.NewKey(t, "k1"))

	_, err := verifier.Verify(idptest.NewKey(t, "k1").Sign(t, idptest.ValidClaims("user-1")))

	require.ErrorIs(t, err, idp.ErrInvalidToken)
}

// 公開鍵を HMAC の共有鍵として使わせる差し替えや、署名なし（none）を受け付けない
func TestVerify_RejectsAlgorithmsOtherThanEdDSA(t *testing.T) {
	key := idptest.NewKey(t, "k1")
	verifier, _ := newVerifier(t, key)
	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, idptest.ValidClaims("user-1"))
	hs.Header["kid"] = key.Kid()
	hsToken, err := hs.SignedString([]byte(ed25519.PublicKey(key.PublicKey())))
	require.NoError(t, err)
	none := jwt.NewWithClaims(jwt.SigningMethodNone, idptest.ValidClaims("user-1"))
	none.Header["kid"] = key.Kid()
	noneToken, err := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = verifier.Verify(hsToken)
	require.ErrorIs(t, err, idp.ErrInvalidToken)
	_, err = verifier.Verify(noneToken)
	require.ErrorIs(t, err, idp.ErrInvalidToken)
}

// studio が鍵を作り直した（secret の漏洩時の手順等）とき、新しい kid の JWT を取り直した JWKS で受け付ける
func TestVerify_RefetchesJWKSForUnknownKid(t *testing.T) {
	oldKey, newKey := idptest.NewKey(t, "old"), idptest.NewKey(t, "new")
	verifier, server := newVerifier(t, oldKey)
	_, err := verifier.Verify(oldKey.Sign(t, idptest.ValidClaims("user-1")))
	require.NoError(t, err)
	before := server.RequestCount()

	server.SetKeys(newKey)
	claims, err := verifier.Verify(newKey.Sign(t, idptest.ValidClaims("user-1")))

	require.NoError(t, err)
	require.Equal(t, "user-1", claims.Subject)
	require.Greater(t, server.RequestCount(), before)
}

// studio が最初の鍵を作った直後（core はまだ鍵を知らない）に、その鍵の JWT が同時に来ても、取り直しの枠を取れなかった方も受け付ける（core#51）
func TestVerify_AcceptsConcurrentTokensWithNewKid(t *testing.T) {
	verifier, server := newVerifier(t)
	key := idptest.NewKey(t, "new")
	server.SetKeys(key)
	token := key.Sign(t, idptest.ValidClaims("user-1"))

	const concurrency = 3
	errs := make(chan error, concurrency)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Go(func() {
			_, err := verifier.Verify(token)
			errs <- err
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
}
