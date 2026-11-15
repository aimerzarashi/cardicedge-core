package item

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type Command interface {
	isItemCommand()
}

type RegisterCommand struct {
	ID           ID
	PublicID     uuid.UUID
	Name         card.Name
	RegisteredBy operator.ID
}

// 公開中にする（準備中・使用不可から）。公開中なら何も変えない
type PublishCommand struct {
	PublishedBy operator.ID
}

// 使用不可にする（公開中から）。使用不可なら何も変えない。準備中からはできない
type RetireCommand struct {
	RetiredBy operator.ID
}

// コストを変える（準備中だけ）。同じ値なら何も変えない
type ChangeCostCommand struct {
	Cost      card.Cost
	ChangedBy operator.ID
}

// 希少度を変える（準備中だけ）。同じ値なら何も変えない
type ChangeRarityCommand struct {
	Rarity    card.Rarity
	ChangedBy operator.ID
}

// 分類を変える（準備中だけ）。同じ値なら何も変えない。今の効果を持てない分類には変えられない（ErrEffectNotAllowed）。
// 持てない効果を消して変える形にしないのは、入れた効果が黙って消えないようにするため
type ChangeCategoryCommand struct {
	Category  card.ItemCategory
	ChangedBy operator.ID
}

// 効果の並びを変える（準備中だけ。変えた後の並びの全体）。分類で持てない効果は ErrEffectNotAllowed。同じ並びなら何も変えない
type ChangeEffectsCommand struct {
	Effects   effect.Effects
	ChangedBy operator.ID
}

func (*RegisterCommand) isItemCommand()       {}
func (*PublishCommand) isItemCommand()        {}
func (*RetireCommand) isItemCommand()         {}
func (*ChangeCostCommand) isItemCommand()     {}
func (*ChangeRarityCommand) isItemCommand()   {}
func (*ChangeCategoryCommand) isItemCommand() {}
func (*ChangeEffectsCommand) isItemCommand()  {}
