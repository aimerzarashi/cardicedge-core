// Package gqltrace は、gqlgen の GraphQL サーバーに OpenTelemetry のスパンを付ける拡張（core#75）。
// 操作ごとのスパンと、リゾルバーのスパンだけを作る。単純なフィールド（構造体の項目を読むだけのもの）は作らない（数が多くノイズになるため）。
//
// gqlgen は公式の OpenTelemetry の拡張を持たない（apollotracing は Apollo の形式のため使えない）。
// サードパーティ（otelgqlgen 等）を入れなかったのは、gqlgen の版に追従する保守の負担を外に持たせるより、
// InterceptOperation・InterceptField の 2 つだけの小さな拡張を自分で持つほうが軽いため。
// studio・game の両方の handler が使う基盤の部品で、サービスの層には属さない
package gqltrace

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "gitlab.com/aimerzarashi/diecardomain/core/internal/platform/gqltrace"

// Extension は gqlgen の handler.Server に `srv.Use(gqltrace.New())` で付ける
type Extension struct {
	tracer trace.Tracer
}

var (
	_ graphql.HandlerExtension     = Extension{}
	_ graphql.OperationInterceptor = Extension{}
	_ graphql.FieldInterceptor     = Extension{}
)

// New は、グローバルのトレーサープロバイダーのトレーサーを使う拡張を作る。
// opts は、テストが記録用のプロバイダーを渡すためのもの
func New(opts ...Option) Extension {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	provider := o.provider
	if provider == nil {
		provider = otel.GetTracerProvider()
	}
	return Extension{tracer: provider.Tracer(tracerName)}
}

type options struct {
	provider trace.TracerProvider
}

type Option func(*options)

func WithTracerProvider(provider trace.TracerProvider) Option {
	return func(o *options) {
		o.provider = provider
	}
}

func (Extension) ExtensionName() string {
	return "OpenTelemetry"
}

func (Extension) Validate(graphql.ExecutableSchema) error {
	return nil
}

// InterceptOperation は操作のスパンを作る。実行は返す ResponseHandler の中で行われるので、スパンはそこで閉じる
// （フィールドのスパンがこのスパンの子になる）
func (e Extension) InterceptOperation(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
	operation := graphql.GetOperationContext(ctx)
	if operation == nil || operation.Operation == nil {
		return next(ctx)
	}
	kind := string(operation.Operation.Operation)
	// リクエストが operationName を送らなくても、クエリに書かれた名前があればそれを使う
	name := operation.OperationName
	if name == "" {
		name = operation.Operation.Name
	}
	spanName := "graphql " + kind
	attrs := []attribute.KeyValue{attribute.String("graphql.operation.type", kind)}
	if name != "" {
		spanName += " " + name
		attrs = append(attrs, attribute.String("graphql.operation.name", name))
	}
	ctx, span := e.tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attrs...))
	handler := next(ctx)
	return func(ctx context.Context) *graphql.Response {
		defer span.End()
		response := handler(ctx)
		if response != nil && len(response.Errors) > 0 {
			span.SetStatus(codes.Error, fmt.Sprintf("%d GraphQL errors", len(response.Errors)))
		}
		return response
	}
}

// InterceptField はリゾルバーのスパンを作る。IsResolver でないフィールドは何もしない
func (e Extension) InterceptField(ctx context.Context, next graphql.Resolver) (any, error) {
	field := graphql.GetFieldContext(ctx)
	if field == nil || !field.IsResolver {
		return next(ctx)
	}
	ctx, span := e.tracer.Start(ctx, field.Object+"."+field.Field.Name, trace.WithAttributes(attribute.String("graphql.field.path", field.Path().String())))
	defer span.End()
	result, err := next(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "resolver failed")
	}
	return result, err
}
