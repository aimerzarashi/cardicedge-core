package telemetry

import (
	"log/slog"
	"sync"
	"time"
)

// errorHandler は OpenTelemetry の内部の誤り（受け手に届かない等）を、interval に 1 回だけ WARN で出す。
// ERROR にしないのは、受け手が止まっていても core は普通に動くため（結合テストは ERROR のログで全体を失敗にする）。
// 出さなかった分の件数は次に出すときに添える
type errorHandler struct {
	logger   *slog.Logger
	interval time.Duration

	mu         sync.Mutex
	last       time.Time
	suppressed int
}

func newErrorHandler(logger *slog.Logger, interval time.Duration) *errorHandler {
	return &errorHandler{logger: logger, interval: interval}
}

func (h *errorHandler) Handle(err error) {
	h.mu.Lock()
	now := time.Now()
	if !h.last.IsZero() && now.Sub(h.last) < h.interval {
		h.suppressed++
		h.mu.Unlock()
		return
	}
	suppressed := h.suppressed
	h.last, h.suppressed = now, 0
	h.mu.Unlock()
	h.logger.Warn("telemetry: OpenTelemetry error", "error", err, "suppressed", suppressed)
}
