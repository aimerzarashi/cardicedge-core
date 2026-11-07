package rdb

import (
	"context"
	"database/sql"

	"go.opentelemetry.io/otel/trace"
)

// traceCommentExecutor は、ctx のスパンの traceparent を SQL のコメントに足してから流す（core#106）。
// PostgreSQL の遅い SQL・実行計画のログに SQL の本文ごと残るので、受け手（Collector）がそこから trace_id を取り出し、
// 遅い SQL からそれを出したリクエストのトレースへ移れる。形は OpenTelemetry の sqlcommenter に合わせる。
// SQL をすべてスパンにする計装（otelsql 等）を入れなかったのは、ほしいのはログとトレースのつながりだけで、
// SQL ごとのスパンはトレースのノイズになるため。otelsql のコメントを付ける機能は試しの扱いでもある
type traceCommentExecutor struct {
	Executor
}

func (e traceCommentExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return e.Executor.ExecContext(ctx, withTraceComment(ctx, query), args...)
}

func (e traceCommentExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return e.Executor.QueryContext(ctx, withTraceComment(ctx, query), args...)
}

func (e traceCommentExecutor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return e.Executor.QueryRowContext(ctx, withTraceComment(ctx, query), args...)
}

// コメントの前で改行するのは、SQL が行コメント（--）で終わっていても、足したコメントがその中に入らないようにするため
func withTraceComment(ctx context.Context, query string) string {
	span := trace.SpanContextFromContext(ctx)
	if !span.IsValid() {
		return query
	}
	flags := "00"
	if span.IsSampled() {
		flags = "01"
	}
	return query + "\n/*traceparent='00-" + span.TraceID().String() + "-" + span.SpanID().String() + "-" + flags + "'*/"
}
