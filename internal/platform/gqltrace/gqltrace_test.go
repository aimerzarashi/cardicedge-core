package gqltrace_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler/testserver"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/gqltrace"
)

func newExtension() (gqltrace.Extension, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	return gqltrace.New(gqltrace.WithTracerProvider(provider)), recorder
}

func attributes(span sdktrace.ReadOnlySpan) map[string]string {
	found := map[string]string{}
	for _, kv := range span.Attributes() {
		found[string(kv.Key)] = kv.Value.String()
	}
	return found
}

func TestExtension_CreatesOperationSpan(t *testing.T) {
	extension, recorder := newExtension()
	srv := testserver.New()
	srv.AddTransport(transport.POST{})
	srv.Use(extension)

	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"query":"query Names { name }","operationName":"Names"}`))
	request.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, request)

	require.Equal(t, http.StatusOK, rec.Code)
	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, "graphql query Names", spans[0].Name())
	require.Equal(t, map[string]string{"graphql.operation.type": "query", "graphql.operation.name": "Names"}, attributes(spans[0]))
}

func TestExtension_OperationSpanHasNoNameForAnonymousOperation(t *testing.T) {
	extension, recorder := newExtension()
	srv := testserver.New()
	srv.AddTransport(transport.POST{})
	srv.Use(extension)

	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"query":"mutation { name }"}`))
	request.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(httptest.NewRecorder(), request)

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, "graphql mutation", spans[0].Name())
	require.Equal(t, map[string]string{"graphql.operation.type": "mutation"}, attributes(spans[0]))
}

func resolverContext(ctx context.Context, isResolver bool) context.Context {
	return graphql.WithFieldContext(ctx, &graphql.FieldContext{
		Object:     "Query",
		Field:      graphql.CollectedField{Field: &ast.Field{Name: "cards"}},
		IsResolver: isResolver,
	})
}

func TestExtension_CreatesSpanForResolverField(t *testing.T) {
	extension, recorder := newExtension()
	ctx, parent := sdktrace.NewTracerProvider().Tracer("test").Start(t.Context(), "operation")

	var inner trace.SpanContext
	result, err := extension.InterceptField(resolverContext(ctx, true), func(ctx context.Context) (any, error) {
		inner = trace.SpanContextFromContext(ctx)
		return "ok", nil
	})

	require.NoError(t, err)
	require.Equal(t, "ok", result)
	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, "Query.cards", spans[0].Name())
	require.Equal(t, parent.SpanContext().SpanID(), spans[0].Parent().SpanID())
	require.Equal(t, spans[0].SpanContext().SpanID(), inner.SpanID())
}

func TestExtension_SkipsFieldWithoutResolver(t *testing.T) {
	extension, recorder := newExtension()

	result, err := extension.InterceptField(resolverContext(t.Context(), false), func(context.Context) (any, error) {
		return "ok", nil
	})

	require.NoError(t, err)
	require.Equal(t, "ok", result)
	require.Empty(t, recorder.Ended())
}

func TestExtension_MarksResolverSpanAsErrorWhenResolverFails(t *testing.T) {
	extension, recorder := newExtension()
	failure := errors.New("boom")

	_, err := extension.InterceptField(resolverContext(t.Context(), true), func(context.Context) (any, error) {
		return nil, failure
	})

	require.ErrorIs(t, err, failure)
	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, "Error", spans[0].Status().Code.String())
}
