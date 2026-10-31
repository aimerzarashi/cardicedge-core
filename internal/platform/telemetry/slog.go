package telemetry

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// traceHandler は、ctx のスパンの trace_id・span_id をログに足す。
// OpenTelemetry のログ（otelslog）は ctx から自分でトレースの情報を付けるので、足すのは標準出力に書く側だけ
type traceHandler struct {
	slog.Handler
}

func newTraceHandler(next slog.Handler) slog.Handler {
	return &traceHandler{Handler: next}
}

func (h *traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{Handler: h.Handler.WithGroup(name)}
}
