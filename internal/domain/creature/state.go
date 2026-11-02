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

// 登録した人・日時・名前は持たない（Decide もコマンドも読まず、画面に出す値は read model がイベントから作るため）
type Creature struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	Status   card.Status
}

func (Creature) isCreatureState() {}

func InitialState() State {
	return UnregisteredCreature{}
}
