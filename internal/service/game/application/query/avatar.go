package query

import (
	"context"

	"github.com/google/uuid"
)

type Avatar struct {
	ID   uuid.UUID
	Name string
}

// プレイヤーのアバターを作った順に返す
type ListAvatarsByPlayerFunc func(ctx context.Context, playerID uuid.UUID) ([]Avatar, error)

type AvatarHandler struct {
	listByPlayer ListAvatarsByPlayerFunc
}

func NewAvatarHandler(listByPlayer ListAvatarsByPlayerFunc) *AvatarHandler {
	return &AvatarHandler{listByPlayer: listByPlayer}
}

// 投影が遅れていて 1 件も無いときも、空の一覧を返す（nil にしない。GraphQL の [Avatar!]! は null を返せないため）
func (q *AvatarHandler) ListByPlayer(ctx context.Context, playerID uuid.UUID) ([]Avatar, error) {
	avatars, err := q.listByPlayer(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if avatars == nil {
		return []Avatar{}, nil
	}
	return avatars, nil
}
