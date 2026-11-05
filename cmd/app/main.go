package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/config"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/telemetry"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/wiring"
)

const (
	listenAddr = ":3333"
	// docker compose の stop_grace_period・air の kill_delay より短くする。HTTP を止める時間と、トレース・ログを送り切る時間
	// （telemetryFlushTimeout）の合計がこの中に収まるようにする
	shutdownTimeout = 10 * time.Second
	// 受け手が止まっていると、送り切れずにこの時間まで待つ。HTTP とは別の枠にして、HTTP の待ちが長引いても送る時間を残す
	telemetryFlushTimeout = 3 * time.Second
	// ①の規模（カードは数百件、操作するのはオペレーターだけ）で足りる本数。測ってから変える。
	// DB の接続の上限を設けるときは、上限がこの本数を上回るかを確かめる（rdb.Open）
	projectionWorkerCount = 4
	// 起動時と再接続時は間隔によらずすぐ読むため、間隔で拾うのは失敗した集約の試し直しと、手で入れたやることの行（参照の api と同じ値）
	pollInterval = 30 * time.Second
	// 待てば直る失敗（マスタの未読み込み等）を 10 分ほど待てる値
	policyRetryInterval = time.Minute
	policyMaxFailures   = 10
	// 対戦の進みを、変わらなくても送り直す間隔。何も流れない時間で切る中継（envoy の stream の上限・Cloudflare の Tunnel のおよそ 100 秒）に
	// 切られないよう、それより十分短くする。変えたくなったら環境変数に出す
	matchProgressInterval = 5 * time.Second
)

var (
	version = "none"
)

func main() {
	slog.SetDefault(slog.New(stdoutHandler()))

	// `app healthcheck`: 起動中の自プロセスの /health を確認して終了する（compose の healthcheck 用）
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := checkHealth(context.Background(), "http://127.0.0.1"+listenAddr+"/health"); err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
		return
	}

	// `app schema <入口>`: GraphQL のスキーマを SDL で標準出力に書いて終了する（studio・game が型を生成するため。core#22）。
	// 設定・DB を使わずに動くよう、それらを読む前に分岐する。標準出力はスキーマだけにするため、誤りは標準エラーに書く
	if len(os.Args) > 1 && os.Args[1] == "schema" {
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: app schema <name>")
			os.Exit(2)
		}
		if err := writeSchema(os.Args[2], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	slog.Info("Application shutdown complete")
}

// run はアプリケーションのメイン処理を実行する。os.Exit()は呼ばずエラーを返すことで、
// defer チェーンによるリソース解放を確実にする
func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	tel, err := telemetry.Setup(ctx, version, stdoutHandler())
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(tel.Handler))
	// 最初に登録して最後に動かす: DB やイベントの処理を止める間のログも送り切る。
	// ctx は取り消されている（シグナルで止まる）ので使わない
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), telemetryFlushTimeout)
		defer cancel()
		if err := tel.Shutdown(flushCtx); err != nil {
			slog.Warn("failed to flush telemetry", "error", err)
		}
	}()
	studioDB, err := rdb.Open(ctx, cfg.StudioDBDSN)
	if err != nil {
		return err
	}
	defer studioDB.Close()
	gameDB, err := rdb.Open(ctx, cfg.GameDBDSN)
	if err != nil {
		return err
	}
	defer gameDB.Close()
	studioVerifier, err := idp.NewVerifier(ctx, cfg.Studio.JWKSURL, cfg.Studio.Issuer, cfg.Studio.Audience)
	if err != nil {
		return err
	}
	gameVerifier, err := idp.NewVerifier(ctx, cfg.Game.JWKSURL, cfg.Game.Issuer, cfg.Game.Audience)
	if err != nil {
		return err
	}

	studioConfig := studio.Config{Verifier: studioVerifier, Issuer: cfg.Studio.Issuer}
	gameConfig := game.Config{Verifier: gameVerifier, Issuer: cfg.Game.Issuer, MatchProgressInterval: matchProgressInterval}
	eventDriven := eventdriven.Options{WorkerCount: projectionWorkerCount, PollInterval: pollInterval, RetryInterval: policyRetryInterval, MaxFailures: policyMaxFailures}
	app, err := wiring.Start(ctx, wiring.Config{
		StudioDB:             studioDB,
		StudioDSN:            cfg.StudioDBDSN,
		GameDB:               gameDB,
		GameDSN:              cfg.GameDBDSN,
		Version:              version,
		Studio:               studioConfig,
		Game:                 gameConfig,
		InitialOperatorEmail: cfg.InitialOperatorEmail,
		EventDriven:          eventDriven,
	})
	if err != nil {
		return err
	}
	// serve が HTTP を止めて返った後に止まる（defer は後に登録したものから先に動くため、db.Close より先）
	defer app.Stop()

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
	}

	server := &http.Server{Handler: app.Handler}
	server.RegisterOnShutdown(app.CloseSubscriptions)
	slog.Info("HTTP server started", "addr", ln.Addr().String(), "version", version)

	return serve(ctx, server, ln, shutdownTimeout-telemetryFlushTimeout)
}

func stdoutHandler() slog.Handler {
	return slog.NewJSONHandler(os.Stdout, nil)
}
