package config

import (
	"errors"
	"fmt"
	"os"
)

type Config struct {
	// studio 用・game 用の DB のつなぎ先。最小の構成では同じサーバーの別の DB、最大の構成では別のサーバーを指す（core#33）
	StudioDBDSN string
	GameDBDSN   string
	Studio      Issuer
	Game        Issuer
	// オペレーターが 1 人もいないときに、起動時に登録するオペレーターのメールアドレス
	InitialOperatorEmail string
}

// JWT の発行元（better-auth）ごとの設定。iss・aud は発行元の設定（studio・game の JWT_ISSUER・JWT_AUDIENCE）と一致させる
type Issuer struct {
	JWKSURL  string
	Issuer   string
	Audience string
}

// 未設定を起動時にまとめてエラーにする（既定値で動くと、別の発行元の JWT を受け付ける等の設定の誤りに気づけないため）
func Load() (Config, error) {
	var missing []error
	get := func(name string) string {
		value := os.Getenv(name)
		if value == "" {
			missing = append(missing, fmt.Errorf("環境変数 %s が設定されていません", name))
		}
		return value
	}
	cfg := Config{
		StudioDBDSN: get("STUDIO_DB_DSN"),
		GameDBDSN:   get("GAME_DB_DSN"),
		Studio: Issuer{
			JWKSURL:  get("STUDIO_JWKS_URL"),
			Issuer:   get("STUDIO_JWT_ISSUER"),
			Audience: get("STUDIO_JWT_AUDIENCE"),
		},
		Game: Issuer{
			JWKSURL:  get("GAME_JWKS_URL"),
			Issuer:   get("GAME_JWT_ISSUER"),
			Audience: get("GAME_JWT_AUDIENCE"),
		},
		InitialOperatorEmail: get("STUDIO_INITIAL_OPERATOR_EMAIL"),
	}
	return cfg, errors.Join(missing...)
}
