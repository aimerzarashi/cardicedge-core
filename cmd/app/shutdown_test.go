package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// startServe は handler を載せた serve をバックグラウンドで起動し、URLと終了結果のチャネルを返す。
func startServe(t *testing.T, ctx context.Context, handler http.Handler, timeout time.Duration) (string, <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, &http.Server{Handler: handler}, ln, timeout)
	}()
	return "http://" + ln.Addr().String(), done
}

func TestServe_WaitsForInFlightRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	started := make(chan struct{})
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		io.WriteString(w, "done")
	})
	url, done := startServe(t, ctx, handler, 5*time.Second)

	type result struct {
		body string
		err  error
	}
	resCh := make(chan result, 1)
	go func() {
		resp, err := http.Get(url)
		if err != nil {
			resCh <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		resCh <- result{body: string(b), err: err}
	}()

	<-started
	cancel()

	// 処理中のリクエストが残っている間は終了しない
	select {
	case err := <-done:
		t.Fatalf("serve returned before in-flight request finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	res := <-resCh
	require.NoError(t, res.err)
	require.Equal(t, "done", res.body)
	require.NoError(t, <-done)

	// 停止後は新規接続を受け付けない
	_, err := http.Get(url)
	require.Error(t, err)
}

func TestServe_ReturnsErrorOnShutdownTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	})
	url, done := startServe(t, ctx, handler, 50*time.Millisecond)

	go http.Get(url)
	<-started
	cancel()

	require.ErrorIs(t, <-done, context.DeadlineExceeded)
}
