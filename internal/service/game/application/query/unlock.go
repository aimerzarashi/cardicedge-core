package query

import (
	"context"

	"github.com/google/uuid"
)

// プレイヤーが解放したボードを、解放した順に返す。公開中でないボード（使用不可・準備中・写していない）は飛ばす
type ListUnlockedBoardsByPlayerFunc func(ctx context.Context, playerID uuid.UUID) ([]Board, error)

type UnlockHandler struct {
	listBoardsByPlayer ListUnlockedBoardsByPlayerFunc
}

func NewUnlockHandler(listBoardsByPlayer ListUnlockedBoardsByPlayerFunc) *UnlockHandler {
	return &UnlockHandler{listBoardsByPlayer: listBoardsByPlayer}
}

// 投影が遅れていて解放がまだ無いときも、空の一覧を返す（nil にしない。GraphQL の [Board!]! は null を返せないため）
func (q *UnlockHandler) ListBoardsByPlayer(ctx context.Context, playerID uuid.UUID) ([]Board, error) {
	boards, err := q.listBoardsByPlayer(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if boards == nil {
		return []Board{}, nil
	}
	return boards, nil
}
