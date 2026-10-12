package idp

import (
	"context"
	"errors"
	"fmt"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("idp: invalid token")

// 発行元ごとに作り、iss・aud も必ず確かめる（署名が正しくても、別の発行元・別の宛先の JWT を受け付けないため。core#7）
type Verifier struct {
	keyfunc keyfunc.Keyfunc
	parser  *jwt.Parser
}

// JWKS は keyfunc の既定どおり、1 時間ごとと、知らない kid の JWT が来たとき（5 分に 1 回まで）に取り直す。
// 最初の取得に失敗しても作成は失敗しない（core は cms より先に起動するため）
func NewVerifier(ctx context.Context, jwksURL, issuer, audience string) (*Verifier, error) {
	kf, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("failed to create keyfunc for %s: %w", jwksURL, err)
	}
	return &Verifier{
		keyfunc: kf,
		parser: jwt.NewParser(
			// better-auth の jwt プラグインは EdDSA（Ed25519）で署名する。alg を JWT のヘッダーに任せると、
			// none や HMAC（公開鍵を共有鍵として使わせる）への差し替えを許すため固定する
			jwt.WithValidMethods([]string{"EdDSA"}),
			jwt.WithIssuer(issuer),
			jwt.WithAudience(audience),
			jwt.WithExpirationRequired(),
		),
	}, nil
}

// Email・EmailVerified は、cms が JWT に入れた email・email_verified（OIDC の標準のクレーム名）。
// core が初めてのログインでオペレーターに紐づけるときにだけ使う
type Claims struct {
	Subject       string
	Email         string
	EmailVerified bool
}

type tokenClaims struct {
	jwt.RegisteredClaims
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

func (v *Verifier) Verify(token string) (Claims, error) {
	claims := &tokenClaims{}
	if _, err := v.parser.ParseWithClaims(token, claims, v.keyfunc.Keyfunc); err != nil {
		return Claims{}, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if claims.Subject == "" {
		return Claims{}, fmt.Errorf("%w: sub is empty", ErrInvalidToken)
	}
	return Claims{Subject: claims.Subject, Email: claims.Email, EmailVerified: claims.EmailVerified}, nil
}
