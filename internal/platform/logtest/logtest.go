// Package logtest は、テストで slog のログを貯めて確かめる道具。
package logtest

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

// Record は貯めたログの 1 行。Attrs は項目の名前から値
type Record struct {
	Level   slog.Level
	Message string
	Attrs   map[string]any
}

type Records struct {
	mu   sync.Mutex
	rows []Record
}

// Capture は slog の既定を、ログを貯めるものに差し替える（テストの終わりに戻す）。
// 既定を差し替えるので、並列に動くテスト（t.Parallel）や、同じプロセスのほかのテストのログも混ざる。
// 確かめる側は Message と項目で絞る
func Capture(t *testing.T) *Records {
	t.Helper()
	records := &Records{}
	previous := slog.Default()
	slog.SetDefault(slog.New(&handler{records: records}))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
	return records
}

// Find は message が一致するログを、出た順に返す
func (r *Records) Find(message string) []Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	var found []Record
	for _, row := range r.rows {
		if row.Message == message {
			found = append(found, row)
		}
	}
	return found
}

type handler struct {
	records *Records
	attrs   []slog.Attr
}

func (h *handler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	row := Record{Level: r.Level, Message: r.Message, Attrs: map[string]any{}}
	// 本番のハンドラー（telemetry）と同じく、ctx に有効なトレースがあるときだけ trace_id・span_id を足す
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		row.Attrs["trace_id"] = sc.TraceID().String()
		row.Attrs["span_id"] = sc.SpanID().String()
	}
	for _, a := range h.attrs {
		row.Attrs[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		row.Attrs[a.Key] = a.Value.Any()
		return true
	})
	h.records.mu.Lock()
	h.records.rows = append(h.records.rows, row)
	h.records.mu.Unlock()
	return nil
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &handler{records: h.records, attrs: append(append([]slog.Attr(nil), h.attrs...), attrs...)}
}

func (h *handler) WithGroup(string) slog.Handler {
	return h
}
