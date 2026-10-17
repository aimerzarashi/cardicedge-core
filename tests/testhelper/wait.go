// 結合テストの道具。組み立てたアプリ（internal/wiring）を、テストと同じプロセスで動かして呼ぶ
package testhelper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 参照の api（1 秒）より短いのは、Hub がテストと同じプロセスで動き、問い合わせもインデックスの効く軽いものだから
const pollInterval = 100 * time.Millisecond

// 参照の api が -race の並列で投影が数十秒遅れたのを測って決めた値に合わせる（こちらでは最初の結合テストで測り直す）
const waitTimeout = 60 * time.Second

// 投影のように後から反映されるものを、condition が通るまで問い合わせ直して待つ
func Eventually(t *testing.T, condition func(c *assert.CollectT)) {
	t.Helper()
	require.EventuallyWithT(t, condition, waitTimeout, pollInterval)
}
