package idp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/time/rate"
)

var ErrInvalidToken = errors.New("idp: invalid token")

// 発行元ごとに作り、iss・aud も必ず確かめる（署名が正しくても、別の発行元・別の宛先の JWT を受け付けないため。core#7）
type Verifier struct {
	keyfunc keyfunc.Keyfunc
	parser  *jwt.Parser
}

// JWKS は 1 時間ごとと、知らない kid の JWT が来たときに取り直す。最初の取得に失敗しても作成は失敗しない（core は studio より先に起動するため）。
// 知らない kid での取り直しを keyfunc の既定の 5 分に 1 回にしないのは、新しい鍵の JWT が同時に来たとき、枠を取れなかった方が
// 手元を読み直さずに（待ちの上限 1 分では次の枠に届かず）401 になるため（core#51）。1 秒に 1 回なら、同時に来た N 個目も N−1 秒待てば
// 自分で取り直せる。同時の数が増えて待ちが長くなるなら間隔を縮める。偽の kid で取りに行かせられる回数も毎秒 1 回までに収まる
func NewVerifier(ctx context.Context, jwksURL, issuer, audience string) (*Verifier, error) {
	kf, err := keyfunc.NewDefaultOverrideCtx(ctx, []string{jwksURL}, keyfunc.Override{
		RefreshUnknownKID: rate.NewLimiter(rate.Every(time.Second), 1),
	})
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

// Email・EmailVerified は、studio が JWT に入れた email・email_verified（OIDC の標準のクレーム名）。
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
