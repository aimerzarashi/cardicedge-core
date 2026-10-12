package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

// Creature・Item・Spell で共有する。種類ごとの関数（creature.go 等）から呼び、ID の型はそちらで守る（参照の api と同じ分け方）
func updateCardName(ctx context.Context, id int64, name card.Name) error {
	return updateUniqueKey(ctx, id, card.NameKey(name), card.ErrNameAlreadyInUse)
}
