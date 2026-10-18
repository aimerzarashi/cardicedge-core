package item

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

// 登録にも名前の変更にも使う。カード名は Creature・Spell と共有するため、ほかのカードが同じ名前（正規化後）を使っていれば
// card.ErrNameAlreadyInUse
type UpdateNameFunc func(ctx context.Context, id ID, name card.Name) error
