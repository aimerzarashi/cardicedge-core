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

// 登録した人・日時・名前は持たない（Decide もコマンドも読まず、画面に出す値は read model がイベントから作るため）
type Spell struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	Status   card.Status
}

func (Spell) isSpellState() {}

func InitialState() State {
	return UnregisteredSpell{}
}
