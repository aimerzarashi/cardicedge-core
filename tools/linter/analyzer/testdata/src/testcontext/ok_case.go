package testcontext

import (
	"context"
	"testing"
	"time"
)

// t.Context() の使用: OK
func TestTContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_ = ctx
}

// TestMain は *testing.T を持たないため対象外: OK
func TestMain(m *testing.M) {
	_ = context.Background()
	m.Run()
}

// t を受け取らない関数は対象外: OK
func newBackground() context.Context {
	return context.Background()
}
