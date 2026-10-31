// Package telemetry は、OpenTelemetry のトレースとログの送り出しを組み立てる（core#75）。
//
// 送り先は OpenTelemetry の標準の環境変数（OTEL_EXPORTER_OTLP_ENDPOINT・OTEL_SERVICE_NAME 等）で決め、SDK の読み方に任せる。
// platform/config の「環境変数はすべて必須」の例外で、OTEL_EXPORTER_OTLP_ENDPOINT が無ければ送り出しを作らない。
package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	endpointEnv = "OTEL_EXPORTER_OTLP_ENDPOINT"
	// 受け手に届かないとき、同じ種類の内部の誤りを出す間隔の下限
	errorLogInterval = time.Minute
)

// 伝え方は W3C の TraceContext（traceparent）だけ。baggage は使わない
var propagator = propagation.TraceContext{}

// Telemetry は組み立てた送り出し。Handler を slog の既定にする。
type Telemetry struct {
	// 標準出力に trace_id・span_id を足して書く。送り出しがあれば、同じログを OpenTelemetry のログとしても送る
	Handler  slog.Handler
	shutdown func(context.Context) error
}

// Setup は、標準出力のハンドラー stdout を包んだログのハンドラーと、送り出し（トレース・ログ）を作る。
// OTEL_EXPORTER_OTLP_ENDPOINT が無ければ、送り出しは作らず（何も送らない・何も待たない）、ハンドラーは標準出力だけに書く。
// 送り出しは HTTP（http/protobuf）だけを扱う。OTEL_EXPORTER_OTLP_PROTOCOL を読んで gRPC に切り替える作りにしなかったのは、
// 開発の受け手へ HTTP で届けば足り、gRPC の分だけ依存と設定の組み合わせが増えるため
func Setup(ctx context.Context, version string, stdout slog.Handler) (*Telemetry, error) {
	local := newTraceHandler(stdout)
	otel.SetTextMapPropagator(propagator)
	if os.Getenv(endpointEnv) == "" {
		return &Telemetry{Handler: local, shutdown: func(context.Context) error { return nil }}, nil
	}

	// 受け手に届かない間、内部の誤りが ERROR のログで大量に出ないようにする。
	// 誤りを受け手へ送るログに流すと、届かない誤りがまた誤りを生むので、標準出力だけに書く
	otel.SetErrorHandler(newErrorHandler(slog.New(local), errorLogInterval))

	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(attribute.String("service.version", version)))
	if err != nil {
		return nil, err
	}
	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	logExporter, err := otlploghttp.New(ctx)
	if err != nil {
		return nil, errors.Join(err, traceExporter.Shutdown(ctx))
	}
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExporter), sdktrace.WithResource(res))
	loggerProvider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)), sdklog.WithResource(res))
	otel.SetTracerProvider(tracerProvider)

	bridge := otelslog.NewHandler("gitlab.com/aimerzarashi/diecardomain/core", otelslog.WithLoggerProvider(loggerProvider), otelslog.WithVersion(version))
	return &Telemetry{
		Handler: slog.NewMultiHandler(local, bridge),
		shutdown: func(ctx context.Context) error {
			// トレースを先に流す: ログの送り出しの失敗でトレースを失わないよう、片方が失敗してももう片方は止める
			return errors.Join(tracerProvider.Shutdown(ctx), loggerProvider.Shutdown(ctx))
		},
	}, nil
}

// Shutdown は、溜めたトレースとログを送り切って止める。ctx の期限までに終わらなければ諦める
func (t *Telemetry) Shutdown(ctx context.Context) error {
	return t.shutdown(ctx)
}
