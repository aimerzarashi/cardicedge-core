package telemetry

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// HTTP は、受けた traceparent を引き継いでリクエストごとにスパンを作る。ルーターの外側に付ける。
// スパンの名前に chi のルートのパターンを使わないのは、外側からは決まらないため（入口は数が少なく、パスをそのまま使っても増えない）。
// /health は compose の healthcheck が数秒おきに呼ぶので、スパンにしない（ノイズになるため）。
// opts は、テストが記録用のプロバイダーを渡すためのもの
func HTTP(next http.Handler, opts ...otelhttp.Option) http.Handler {
	defaults := []otelhttp.Option{
		otelhttp.WithPropagators(propagator),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != "/health"
		}),
	}
	return otelhttp.NewHandler(next, "http", append(defaults, opts...)...)
}
