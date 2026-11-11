package creature

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

type State interface {
	isCreatureState()
}

type UnregisteredCreature struct{}

func (UnregisteredCreature) isCreatureState() {}

// 登録した人・日時・名前は持たない（Decide もコマンドも読まず、画面に出す値は read model がイベントから作るため）。
// 性能は、同じ値への変更でイベントを作らないために持つ（ボードのレイアウトと同じ）
type Creature struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	Status   card.Status
	Cost     card.Cost
	Rarity   card.Rarity
	// 属性・攻撃力・体力・移動
	Elements              Elements
	Attack                Attack
	HitPoints             HitPoints
	ItemRestrictions      ItemRestrictions
	PlacementRestrictions PlacementRestrictions
	Abilities             Abilities
}

func (Creature) isCreatureState() {}

func InitialState() State {
	return UnregisteredCreature{}
}
