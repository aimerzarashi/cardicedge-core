package idptest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// better-auth の jwt プラグイン（cms）と同じ形の JWT・JWKS を、テストで作るための道具

const (
	Issuer   = "diecardomain-cms"
	Audience = "diecardomain-core"
)

type Key struct {
	kid     string
	private ed25519.PrivateKey
}

func NewKey(t testing.TB, kid string) Key {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return Key{kid: kid, private: private}
}

func (k Key) Kid() string { return k.kid }

func (k Key) PublicKey() ed25519.PublicKey { return k.private.Public().(ed25519.PublicKey) }

// cms の JWT の中身（登録済みのクレームと、definePayload で入れる email・email_verified）
type Claims struct {
	jwt.RegisteredClaims
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"email_verified,omitempty"`
}

func (k Key) Sign(t testing.TB, claims Claims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = k.kid
	signed, err := token.SignedString(k.private)
	require.NoError(t, err)
	return signed
}

// ValidClaims は cms の JWT と同じ中身（sub・iss・aud・iat・exp。有効期限 15 分）。メールアドレスは入れない
func ValidClaims(subject string) Claims {
	return Claims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer:    Issuer,
		Subject:   subject,
		Audience:  jwt.ClaimStrings{Audience},
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
	}}
}

// VerifiedClaims は ValidClaims に、確認済みのメールアドレスを入れたもの
func VerifiedClaims(subject, email string) Claims {
	claims := ValidClaims(subject)
	claims.Email = email
	claims.EmailVerified = true
	return claims
}

// JWKSServer は cms の /api/auth/jwks と同じ形（OKP・Ed25519）の JWKS を返す。返す鍵はテストの途中で差し替えられる
type JWKSServer struct {
	URL      string
	mu       sync.Mutex
	keys     []Key
	requests int
}

func NewJWKSServer(t testing.TB, keys ...Key) *JWKSServer {
	t.Helper()
	s := &JWKSServer{keys: keys}
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)
	s.URL = server.URL
	return s
}

func (s *JWKSServer) SetKeys(keys ...Key) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys = keys
}

func (s *JWKSServer) RequestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests
}

func (s *JWKSServer) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests++
	keys := []map[string]string{}
	for _, k := range s.keys {
		keys = append(keys, map[string]string{
			"kty": "OKP", "crv": "Ed25519", "alg": "EdDSA", "kid": k.kid,
			"x": base64.RawURLEncoding.EncodeToString(k.PublicKey()),
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
}
