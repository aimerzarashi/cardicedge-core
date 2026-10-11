package testcontext

import (
	"context"
	stdctx "context"
	"testing"
	"time"
)

// テスト関数直下: NG
func TestDirect(t *testing.T) {
	_ = context.Background() // want "use t.Context\\(\\) instead of context.Background\\(\\) in test functions"
	_ = context.TODO()       // want "use t.Context\\(\\) instead of context.TODO\\(\\) in test functions"
}

// 派生コンテキストの親として使用: NG
func TestDerived(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second) // want "use t.Context\\(\\) instead of context.Background\\(\\) in test functions"
	defer cancel()
	_ = ctx
}

// t.Run のクロージャ内: NG
func TestSubtest(t *testing.T) {
	t.Run("正常系/サブテスト", func(t *testing.T) {
		_ = context.Background() // want "use t.Context\\(\\) instead of context.Background\\(\\) in test functions"
	})
}

// t を引数に持たないクロージャでも外側のスコープに t がある: NG
func TestGoroutine(t *testing.T) {
	go func() {
		_ = context.Background() // want "use t.Context\\(\\) instead of context.Background\\(\\) in test functions"
	}()
}

// t を受け取るヘルパー関数: NG
func setupHelper(t *testing.T) {
	t.Helper()
	_ = context.Background() // want "use t.Context\\(\\) instead of context.Background\\(\\) in test functions"
}

// testing.TB を受け取るヘルパー関数: NG
func setupTBHelper(tb testing.TB) {
	tb.Helper()
	_ = context.Background() // want "use t.Context\\(\\) instead of context.Background\\(\\) in test functions"
}

// ベンチマーク: NG
func BenchmarkDirect(b *testing.B) {
	_ = context.Background() // want "use t.Context\\(\\) instead of context.Background\\(\\) in test functions"
}

// importエイリアス経由: NG
func TestAliased(t *testing.T) {
	_ = stdctx.Background() // want "use t.Context\\(\\) instead of context.Background\\(\\) in test functions"
}
