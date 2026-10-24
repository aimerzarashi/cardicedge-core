package query

import (
	"context"

	"github.com/google/uuid"
)

// 値は GraphQL の StarterGrantStatus と同じ
type StarterGrantStatus string

const (
	StarterGrantInProgress StarterGrantStatus = "IN_PROGRESS"
	StarterGrantCompleted  StarterGrantStatus = "COMPLETED"
)

// 行が無い、または配布の状態がまだ書かれていなければ found は false
type GetStarterGrantStatusFunc func(ctx context.Context, playerID uuid.UUID) (status StarterGrantStatus, found bool, err error)

type PlayerHandler struct {
	getStarterGrantStatus GetStarterGrantStatusFunc
}

func NewPlayerHandler(getStarterGrantStatus GetStarterGrantStatusFunc) *PlayerHandler {
	return &PlayerHandler{getStarterGrantStatus: getStarterGrantStatus}
}

// 行が無いこと（投影の遅れ）と、配布が始まっていないことは、画面にとって「まだ届いていない」で同じなので、IN_PROGRESS にする
func (q *PlayerHandler) StarterGrant(ctx context.Context, playerID uuid.UUID) (StarterGrantStatus, error) {
	status, found, err := q.getStarterGrantStatus(ctx, playerID)
	if err != nil || !found {
		return StarterGrantInProgress, err
	}
	return status, nil
}
