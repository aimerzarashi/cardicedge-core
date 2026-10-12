package config

import (
	"errors"
	"fmt"
	"os"
)

type Config struct {
	DBDSN string
	CMS   Issuer
	// オペレーターが 1 人もいないときに、起動時に登録するオペレーターのメールアドレス
	InitialOperatorEmail string
}

// JWT の発行元（better-auth）ごとの設定。iss・aud は発行元の設定（cms の JWT_ISSUER・JWT_AUDIENCE）と一致させる
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
		DBDSN: get("DB_DSN"),
		CMS: Issuer{
			JWKSURL:  get("CMS_JWKS_URL"),
			Issuer:   get("CMS_JWT_ISSUER"),
			Audience: get("CMS_JWT_AUDIENCE"),
		},
		InitialOperatorEmail: get("CMS_INITIAL_OPERATOR_EMAIL"),
	}
	return cfg, errors.Join(missing...)
}
