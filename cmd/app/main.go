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
)

const (
	listenAddr = ":3333"
	// docker compose の stop_grace_period・air の kill_delay より短くする
	shutdownTimeout = 10 * time.Second
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// `app migrate`: 未適用のマイグレーションを適用して終了する（core の起動前に1回実行する）
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := runMigrate(ctx); err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
		return
	}

	if err := run(ctx); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	slog.Info("Application shutdown complete")
}

// run はアプリケーションのメイン処理を実行する。os.Exit()は呼ばずエラーを返すことで、
// defer チェーンによるリソース解放を確実にする
func run(ctx context.Context) error {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
	}

	server := &http.Server{Handler: newRouter()}
	slog.Info("HTTP server started", "addr", ln.Addr().String(), "version", version)

	return serve(ctx, server, ln, shutdownTimeout)
}
