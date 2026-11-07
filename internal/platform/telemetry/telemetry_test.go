package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/telemetry"
)

// 受け手の代わり。届いた OTLP の呼び出しのパスを貯める
type receiver struct {
	server *httptest.Server
	mu     sync.Mutex
	paths  []string
}

func startReceiver(t *testing.T) *receiver {
	t.Helper()
	r := &receiver{}
	r.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.paths = append(r.paths, req.URL.Path)
		r.mu.Unlock()
	}))
	t.Cleanup(r.server.Close)
	return r
}

func (r *receiver) received() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...)
}

// Setup が差し替えるグローバルを、テストの終わりに戻す
func restoreGlobals(t *testing.T) {
	t.Helper()
	tracerProvider := otel.GetTracerProvider()
	meterProvider := otel.GetMeterProvider()
	propagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(tracerProvider)
		otel.SetMeterProvider(meterProvider)
		otel.SetTextMapPropagator(propagator)
		otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) {}))
	})
}

func shutdown(t *testing.T, tel *telemetry.Telemetry) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, tel.Shutdown(ctx))
}

func TestSetup_SendsNothingWithoutEndpoint(t *testing.T) {
	restoreGlobals(t)
	rcv := startReceiver(t)
	// 信号ごとの送り先があっても、共通の送り先が無ければ送り出しを作らない
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", rcv.server.URL+"/v1/traces")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", rcv.server.URL+"/v1/logs")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", rcv.server.URL+"/v1/metrics")

	var out bytes.Buffer
	tel, err := telemetry.Setup(t.Context(), "test", slog.NewJSONHandler(&out, nil))
	require.NoError(t, err)
	slog.New(tel.Handler).InfoContext(t.Context(), "hello")
	shutdown(t, tel)

	require.Contains(t, out.String(), `"msg":"hello"`)
	require.Empty(t, rcv.received())
}

func TestSetup_SendsTracesLogsAndMetricsWithEndpoint(t *testing.T) {
	restoreGlobals(t)
	rcv := startReceiver(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", rcv.server.URL)
	t.Setenv("OTEL_SERVICE_NAME", "core-test")

	var out bytes.Buffer
	tel, err := telemetry.Setup(t.Context(), "test", slog.NewJSONHandler(&out, nil))
	require.NoError(t, err)
	ctx, span := otel.Tracer("test").Start(t.Context(), "work")
	slog.New(tel.Handler).InfoContext(ctx, "hello")
	span.End()
	shutdown(t, tel)

	require.ElementsMatch(t, []string{"/v1/traces", "/v1/logs", "/v1/metrics"}, rcv.received())
	require.Contains(t, out.String(), span.SpanContext().TraceID().String())
}

func TestSetup_LogsTraceIDInsideSpan(t *testing.T) {
	restoreGlobals(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	var out bytes.Buffer
	tel, err := telemetry.Setup(t.Context(), "test", slog.NewJSONHandler(&out, nil))
	require.NoError(t, err)
	logger := slog.New(tel.Handler)

	provider := sdktrace.NewTracerProvider()
	ctx, span := provider.Tracer("test").Start(t.Context(), "work")
	logger.InfoContext(ctx, "inside")
	span.End()
	logger.InfoContext(t.Context(), "outside")

	var inside, outside map[string]any
	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	require.Len(t, lines, 2)
	require.NoError(t, json.Unmarshal(lines[0], &inside))
	require.NoError(t, json.Unmarshal(lines[1], &outside))
	require.Equal(t, span.SpanContext().TraceID().String(), inside["trace_id"])
	require.Equal(t, span.SpanContext().SpanID().String(), inside["span_id"])
	require.NotContains(t, outside, "trace_id")
	require.NotContains(t, outside, "span_id")
}

func TestSetup_KeepsTraceIDThroughWith(t *testing.T) {
	restoreGlobals(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	var out bytes.Buffer
	tel, err := telemetry.Setup(t.Context(), "test", slog.NewJSONHandler(&out, nil))
	require.NoError(t, err)

	ctx, span := sdktrace.NewTracerProvider().Tracer("test").Start(t.Context(), "work")
	slog.New(tel.Handler).With("component", "x").WithGroup("g").InfoContext(ctx, "hello")

	require.Contains(t, out.String(), span.SpanContext().TraceID().String())
}

func TestErrorHandler_LogsOncePerInterval(t *testing.T) {
	var out bytes.Buffer
	handler := telemetry.NewErrorHandler(slog.New(slog.NewJSONHandler(&out, nil)), time.Hour)

	handler.Handle(context.DeadlineExceeded)
	handler.Handle(context.DeadlineExceeded)
	handler.Handle(context.DeadlineExceeded)

	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	require.Len(t, lines, 1)
	var row map[string]any
	require.NoError(t, json.Unmarshal(lines[0], &row))
	require.Equal(t, "WARN", row["level"])
}

func TestErrorHandler_ReportsSuppressedCount(t *testing.T) {
	var out bytes.Buffer
	handler := telemetry.NewErrorHandler(slog.New(slog.NewJSONHandler(&out, nil)), 20*time.Millisecond)

	handler.Handle(context.DeadlineExceeded)
	handler.Handle(context.DeadlineExceeded)
	time.Sleep(40 * time.Millisecond)
	handler.Handle(context.DeadlineExceeded)

	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	require.Len(t, lines, 2)
	var row map[string]any
	require.NoError(t, json.Unmarshal(lines[1], &row))
	require.EqualValues(t, 1, row["suppressed"])
}
