package rdb_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

func withSpan(t *testing.T, ctx context.Context) context.Context {
	t.Helper()
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(t, err)
	span := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled})
	return trace.ContextWithSpanContext(ctx, span)
}

// サーバーが受け取った文を current_query() で読み返す
func receivedQuery(t *testing.T, ctx context.Context, query string) string {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var received string
	require.NoError(t, exec.QueryRowContext(ctx, query).Scan(&received))
	return received
}

func TestGetExecutor_AddsTraceparentOfSpanToSQL(t *testing.T) {
	ctx := withSpan(t, rdbtest.Tx(t))

	received := receivedQuery(t, ctx, "SELECT current_query()")

	require.Equal(t, "SELECT current_query()\n/*traceparent='00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01'*/", received)
}

func TestGetExecutor_LeavesSQLWithoutSpanUnchanged(t *testing.T) {
	received := receivedQuery(t, rdbtest.Tx(t), "SELECT current_query()")

	require.Equal(t, "SELECT current_query()", received)
}

func TestGetExecutor_KeepsTraceparentAfterLineComment(t *testing.T) {
	ctx := withSpan(t, rdbtest.Tx(t))

	received := receivedQuery(t, ctx, "SELECT current_query() -- note")

	require.Contains(t, received, "\n/*traceparent='00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01'*/")
}
