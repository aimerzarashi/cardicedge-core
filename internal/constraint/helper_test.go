package constraint_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
)

var cardNameSuffixes sync.Map

// 開発用の DB に残っているカードの名前とぶつからないよう、テストごとに UUID を付ける（同じテストの中では同じ値）
func cardName(t *testing.T, base string) string {
	t.Helper()
	suffix, _ := cardNameSuffixes.LoadOrStore(t.Name(), uuid.NewString())
	return fmt.Sprintf("%s-%s", base, suffix)
}
