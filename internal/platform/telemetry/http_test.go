package telemetry_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/telemetry"
)

const (
	traceID     = "4bf92f3577b34da6a3ce929d0e0e4736"
	parentSpan  = "00f067aa0ba902b7"
	traceparent = "00-" + traceID + "-" + parentSpan + "-01"
)

func serve(t *testing.T, request *http.Request) (*tracetest.SpanRecorder, trace.SpanContext) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	var seen trace.SpanContext
	handler := telemetry.HTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = trace.SpanContextFromContext(r.Context())
	}), otelhttp.WithTracerProvider(provider))

	handler.ServeHTTP(httptest.NewRecorder(), request)
	return recorder, seen
}

func TestHTTP_ContinuesIncomingTraceparent(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/studio/graphql", nil)
	request.Header.Set("traceparent", traceparent)

	recorder, seen := serve(t, request)

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, "POST /studio/graphql", spans[0].Name())
	require.Equal(t, traceID, spans[0].SpanContext().TraceID().String())
	require.Equal(t, parentSpan, spans[0].Parent().SpanID().String())
	require.Equal(t, spans[0].SpanContext().SpanID(), seen.SpanID())
}

func TestHTTP_StartsNewTraceWithoutTraceparent(t *testing.T) {
	recorder, _ := serve(t, httptest.NewRequest(http.MethodGet, "/version", nil))

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.False(t, spans[0].Parent().IsValid())
}

func TestHTTP_SkipsHealthCheck(t *testing.T) {
	recorder, _ := serve(t, httptest.NewRequest(http.MethodGet, "/health", nil))

	require.Empty(t, recorder.Ended())
}
