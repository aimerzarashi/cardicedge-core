package testhelper

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// アプリが出した ERROR のログ。投影の失敗はログに出るだけで、待つ側のテストからは時間切れとしか見えないため、
// 1 件でも出たら実行全体を失敗にする（並列のテストが Hub を共有するため、どのテストが起こしたかは見分けられない）
type ErrorLogs struct {
	mu      sync.Mutex
	records []string
}

// slog の既定を、標準エラーに書きつつ ERROR を貯めるものに差し替える
func RecordErrorLogs() *ErrorLogs {
	logs := &ErrorLogs{}
	slog.SetDefault(slog.New(&errorRecorder{Handler: slog.NewTextHandler(os.Stderr, nil), logs: logs}))
	return logs
}

func (l *ErrorLogs) Records() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.records...)
}

type errorRecorder struct {
	slog.Handler
	logs  *ErrorLogs
	attrs []slog.Attr
}

func (h *errorRecorder) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError {
		var b strings.Builder
		b.WriteString(r.Message)
		for _, a := range h.attrs {
			fmt.Fprintf(&b, " %s", a)
		}
		r.Attrs(func(a slog.Attr) bool {
			fmt.Fprintf(&b, " %s", a)
			return true
		})
		h.logs.mu.Lock()
		h.logs.records = append(h.logs.records, b.String())
		h.logs.mu.Unlock()
	}
	return h.Handler.Handle(ctx, r)
}

func (h *errorRecorder) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &errorRecorder{Handler: h.Handler.WithAttrs(attrs), logs: h.logs, attrs: append(append([]slog.Attr(nil), h.attrs...), attrs...)}
}

func (h *errorRecorder) WithGroup(name string) slog.Handler {
	return &errorRecorder{Handler: h.Handler.WithGroup(name), logs: h.logs, attrs: h.attrs}
}
