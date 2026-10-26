package eventdriven

import (
	"context"
	"log/slog"
	"time"

	"github.com/lib/pq"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// 起動時にすぐ読む（止まっている間に入ったイベントを拾うため）。その後は間隔ごとと、再接続の合図で読む。
// 合図で読んだら間隔を数え直す（合図の直後に間隔の分で続けて読まないため）
func (h *Hub) poll(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-h.pollNow:
			timer.Stop()
		}
		h.pollOnce(ctx)
		timer.Reset(h.options.PollInterval)
	}
}

// やることの表には遅れている集約しか無いため、読む量は遅れの件数に比例する（イベントの総数によらない）。
// 件数に上限を付けないのは、毎回同じ順で先頭から取ると、失敗し続ける行が先頭にあるとき後ろの行が拾われないため
const findPendingQuery = `
	SELECT subscriber_id, entity_type_id, entity_id
	FROM event_subscriber_pending
	WHERE subscriber_id = ANY($1)
`

func (h *Hub) pollOnce(ctx context.Context) {
	if err := h.enqueuePending(ctx); err != nil && ctx.Err() == nil {
		// 安全網が黙って止まらないよう、失敗はログに出す（参照の api はコメントに反して捨てている）
		slog.ErrorContext(ctx, "eventdriven: polling pending projections failed", "error", err)
	}
}

func (h *Hub) enqueuePending(ctx context.Context) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	subscriberIDs := make([]int64, 0, len(h.registered))
	for id := range h.registered {
		subscriberIDs = append(subscriberIDs, int64(id))
	}
	rows, err := exec.QueryContext(ctx, findPendingQuery, pq.Array(subscriberIDs))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var subscriberID, entityTypeID int16
		var entityID int64
		if err := rows.Scan(&subscriberID, &entityTypeID, &entityID); err != nil {
			return err
		}
		h.scheduler.enqueue(workKey{subscriberID: subscriberID, entityTypeID: domaindef.NewEntityTypeID(entityTypeID), entityID: entityID})
	}
	return rows.Err()
}
