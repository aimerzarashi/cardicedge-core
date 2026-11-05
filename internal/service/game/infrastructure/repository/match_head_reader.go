package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

// 対戦の最新のイベント（版・起きた時刻・戦利品の受け取りが済んだか）を domain_events の最後の 1 行から読む。
// 集約を畳み込まないのは、進みの送り直しが間隔ごと・購読者ごとに走るため（集約のイベントすべてを読み直さない）
type MatchHeadReader struct{}

func NewMatchHeadReader() *MatchHeadReader {
	return &MatchHeadReader{}
}

func (r *MatchHeadReader) Load(ctx context.Context, id match.ID) (query.MatchHead, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.MatchHead{}, err
	}
	var (
		head      query.MatchHead
		eventName string
	)
	err = exec.QueryRowContext(ctx, `
		SELECT e.aggregate_version, e.occurred_at, t.name
		FROM domain_events e
		JOIN domain_event_types t ON t.id = e.event_type_id
		WHERE e.entity_type_id = (SELECT id FROM domain_entity_types WHERE name = $1) AND e.entity_id = $2
		ORDER BY e.aggregate_version DESC
		LIMIT 1`, string(id.EntityType()), id.Int64()).Scan(&head.Revision, &head.OccurredAt, &eventName)
	if errors.Is(err, sql.ErrNoRows) {
		return query.MatchHead{}, fmt.Errorf("match %d has no events", id.Int64())
	}
	if err != nil {
		return query.MatchHead{}, err
	}
	head.RewardsCompleted = eventName == (&match.RewardsCompletedEvent{}).EventType().String()
	return head, nil
}
