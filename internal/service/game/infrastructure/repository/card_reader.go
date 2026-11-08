package repository

import (
	"context"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

// 写した cards（make publish-master が、公開中・使用不可のカードだけを写した表）から 1 枚読む。
// 使用不可を除かないのは、プレイヤーがすでに持っていることがあり、詳細を見られなくなるのを避けるため
type CardReader struct{}

func NewCardReader() *CardReader {
	return &CardReader{}
}

// 無い（準備中で写していない・カードでない ID）なら query.ErrCardNotFound。性能が無い行は読み出しの誤り
func (r *CardReader) Get(ctx context.Context, id uuid.UUID) (query.Card, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.Card{}, err
	}
	cards, err := loadCards(ctx, exec, []uuid.UUID{id})
	if err != nil {
		return query.Card{}, err
	}
	loaded, ok := cards[id]
	if !ok {
		return query.Card{}, query.ErrCardNotFound
	}
	return toCard(loaded.card), nil
}
