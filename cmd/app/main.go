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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/wiring"
)

const (
	listenAddr = ":3333"
	// docker compose の stop_grace_period・air の kill_delay より短くする
	shutdownTimeout = 10 * time.Second
	// ①の規模（カードは数百件、操作するのはオペレーターだけ）で足りる本数。測ってから変える。
	// DB の接続の上限を設けるときは、上限がこの本数を上回るかを確かめる（rdb.Open）
	projectionWorkerCount = 4
	// 起動時と再接続時は間隔によらずすぐ読むため、間隔で拾うのは失敗した集約の試し直しと、手で入れたやることの行（参照の api と同じ値）
	pollInterval = 30 * time.Second
)

var (
	version = "none"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

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
	verifier, err := idp.NewVerifier(ctx, cfg.Studio.JWKSURL, cfg.Studio.Issuer, cfg.Studio.Audience)
	if err != nil {
		return err
	}

	studioConfig := studio.Config{Verifier: verifier, Issuer: cfg.Studio.Issuer}
	eventDriven := eventdriven.Options{WorkerCount: projectionWorkerCount, PollInterval: pollInterval}
	app, err := wiring.Start(ctx, wiring.Config{
		StudioDB:             studioDB,
		StudioDSN:            cfg.StudioDBDSN,
		GameDB:               gameDB,
		Version:              version,
		Studio:               studioConfig,
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
	slog.Info("HTTP server started", "addr", ln.Addr().String(), "version", version)

	return serve(ctx, server, ln, shutdownTimeout)
}
