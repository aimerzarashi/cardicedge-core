package creature

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type Command interface {
	isCreatureCommand()
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

// 属性を変える（準備中だけ）。同じ集合なら何も変えない
type ChangeElementsCommand struct {
	Elements  Elements
	ChangedBy operator.ID
}

// 攻撃力と体力を変える（準備中だけ）。どちらも同じ値なら何も変えない。画面で 2 つを並べて直すため、1 つの操作にまとめる
type ChangeStatsCommand struct {
	Attack    Attack
	HitPoints HitPoints
	ChangedBy operator.ID
}

// 移動を変える（準備中だけ）。同じ値なら何も変えない。AT・HP とは別の操作にする（2026-11-10 オーナー）
type ChangeMovementCommand struct {
	Movement  Movement
	ChangedBy operator.ID
}

type ChangeItemRestrictionsCommand struct {
	ItemRestrictions ItemRestrictions
	ChangedBy        operator.ID
}

type ChangePlacementRestrictionsCommand struct {
	PlacementRestrictions PlacementRestrictions
	ChangedBy             operator.ID
}

func (*RegisterCommand) isCreatureCommand()                    {}
func (*PublishCommand) isCreatureCommand()                     {}
func (*RetireCommand) isCreatureCommand()                      {}
func (*ChangeCostCommand) isCreatureCommand()                  {}
func (*ChangeRarityCommand) isCreatureCommand()                {}
func (*ChangeElementsCommand) isCreatureCommand()              {}
func (*ChangeStatsCommand) isCreatureCommand()                 {}
func (*ChangeMovementCommand) isCreatureCommand()              {}
func (*ChangeItemRestrictionsCommand) isCreatureCommand()      {}
func (*ChangePlacementRestrictionsCommand) isCreatureCommand() {}
