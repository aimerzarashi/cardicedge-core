package eventstore

import (
	"context"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// グローバルの propagator（otel.SetTextMapPropagator）は使わない。設定の有無で保存する形が変わらないよう、W3C の TraceContext に固定する
var traceContext = propagation.TraceContext{}

// ctx の有効なトレースを traceparent の文字列にする。トレースが無ければ空（列は NULL にする）
func traceparentOf(ctx context.Context) string {
	if !trace.SpanContextFromContext(ctx).IsValid() {
		return ""
	}
	carrier := propagation.MapCarrier{}
	traceContext.Inject(ctx, carrier)
	return carrier.Get("traceparent")
}

// 保存された traceparent をトレースの文脈に戻す。空・読めない文字列は、トレースが無いものとして扱う
// （処理のログに trace_id が付かないだけで、投影・ポリシーの処理は止めない）
func spanContextOf(traceparent string) trace.SpanContext {
	if traceparent == "" {
		return trace.SpanContext{}
	}
	ctx := traceContext.Extract(context.Background(), propagation.MapCarrier{"traceparent": traceparent})
	return trace.SpanContextFromContext(ctx)
}
