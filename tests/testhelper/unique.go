package testhelper

import (
	"testing"

	"github.com/google/uuid"
)

// 並列のテストが同じ DB を使うため、テストで作るもの（名前・メールアドレス・subject）には、ほかのテストとぶつからない値を付ける
func UniqueID(t *testing.T) string {
	t.Helper()
	return uuid.NewString()
}
