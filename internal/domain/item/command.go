package item

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
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

// 分類を変える（準備中だけ）。同じ値なら何も変えない
type ChangeCategoryCommand struct {
	Category  Category
	ChangedBy operator.ID
}

// 戦闘中の AT・HP の増減を変える（準備中だけ。2 つは一緒に変える）。どちらも同じ値なら何も変えない
type ChangeStatModifiersCommand struct {
	AttackModifier    AttackModifier
	HitPointsModifier HitPointsModifier
	ChangedBy         operator.ID
}

func (*RegisterCommand) isItemCommand()            {}
func (*PublishCommand) isItemCommand()             {}
func (*RetireCommand) isItemCommand()              {}
func (*ChangeCostCommand) isItemCommand()          {}
func (*ChangeRarityCommand) isItemCommand()        {}
func (*ChangeCategoryCommand) isItemCommand()      {}
func (*ChangeStatModifiersCommand) isItemCommand() {}
