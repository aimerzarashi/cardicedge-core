package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// serve は ctx がキャンセルされるまで server を動かし、その後グレースフルシャットダウンする。
// 新規接続の受付を止め、処理中のリクエストが終わるのを timeout まで待つ。
func serve(ctx context.Context, server *http.Server, ln net.Listener, timeout time.Duration) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(ln)
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("HTTP server error: %w", err)
	case <-ctx.Done():
		slog.Info("Shutdown signal received, initiating graceful shutdown")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("HTTP server shutdown error: %w", err)
	}

	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("HTTP server error: %w", err)
	}
	return nil
}
