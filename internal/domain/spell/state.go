package spell

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

type State interface {
	isSpellState()
}

type UnregisteredSpell struct{}

func (UnregisteredSpell) isSpellState() {}

// 登録した人・日時・名前は持たない（Decide もコマンドも読まず、画面に出す値は read model がイベントから作るため）。
// 性能は、同じ値への変更でイベントを作らないために持つ（ボードのレイアウトと同じ）
type Spell struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	Status   card.Status
	Cost     card.Cost
	Rarity   card.Rarity
}

func (Spell) isSpellState() {}

func InitialState() State {
	return UnregisteredSpell{}
}
