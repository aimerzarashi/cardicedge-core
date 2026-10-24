package testhelper

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/wiring"
)

// 結合テストの TestMain が、本番と同じ組み立て（internal/wiring）で起動したアプリ。studio・game の入口の両方を持つ
// （入口ごとに組み立てを分けると、本番と違う組み立てでテストすることになるため）
type App struct {
	// httptest のサーバーの URL（入口のパスは付けない）
	URL      string
	StudioDB *sql.DB
	GameDB   *sql.DB
	// studio・game の JWKS が返す鍵。studio の JWT は idptest.ValidClaims、game の JWT は iss を idptest.GameIssuer に変えて作る
	Key     idptest.Key
	cleanup []func()
}

// 複製した DB（studio 用・game 用）と JWKS のサーバーを用意して起動する。止めるのは Close
func StartApp(ctx context.Context) (*App, error) {
	a := &App{}
	if err := a.start(ctx); err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}

func (a *App) start(ctx context.Context) error {
	studioDB, studioDSN, dropStudio, err := rdbtest.CloneDatabase(ctx)
	if err != nil {
		return err
	}
	a.cleanup = append(a.cleanup, dropStudio)
	gameDB, gameDSN, dropGame, err := rdbtest.CloneDatabase(ctx)
	if err != nil {
		return err
	}
	a.cleanup = append(a.cleanup, dropGame)
	key, err := idptest.GenerateKey("k1")
	if err != nil {
		return err
	}
	jwks, closeJWKS := idptest.StartJWKSServer(key)
	a.cleanup = append(a.cleanup, closeJWKS)
	studioVerifier, err := idp.NewVerifier(ctx, jwks.URL, idptest.Issuer, idptest.Audience)
	if err != nil {
		return err
	}
	gameVerifier, err := idp.NewVerifier(ctx, jwks.URL, idptest.GameIssuer, idptest.Audience)
	if err != nil {
		return err
	}
	studioConfig := studio.Config{Verifier: studioVerifier, Issuer: idptest.Issuer}
	gameConfig := game.Config{Verifier: gameVerifier, Issuer: idptest.GameIssuer}
	// ポリシーの待てば直る失敗（マスタの公開前に登録した場合等）を短い間隔で確かめるため、やり直しの間隔を短くし、
	// その分デッドレターにならないよう回数を増やす（本番の設定とは別）
	eventDriven := eventdriven.Options{WorkerCount: 4, PollInterval: time.Second, RetryInterval: 100 * time.Millisecond, MaxFailures: 100}
	app, err := wiring.Start(ctx, wiring.Config{
		StudioDB:             studioDB,
		StudioDSN:            studioDSN,
		GameDB:               gameDB,
		GameDSN:              gameDSN,
		Version:              "test",
		Studio:               studioConfig,
		Game:                 gameConfig,
		InitialOperatorEmail: "initial@example.com",
		EventDriven:          eventDriven,
	})
	if err != nil {
		return err
	}
	a.cleanup = append(a.cleanup, app.Stop)
	server := httptest.NewServer(app.Handler)
	// HTTP を止めてから Hub を止める（cmd/app と同じ順）
	a.cleanup = append(a.cleanup, server.Close)
	a.URL, a.StudioDB, a.GameDB, a.Key = server.URL, studioDB, gameDB, key
	return nil
}

// 用意した順の逆に止める
func (a *App) Close() {
	for i := len(a.cleanup) - 1; i >= 0; i-- {
		a.cleanup[i]()
	}
	a.cleanup = nil
}
