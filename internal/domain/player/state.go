package player

import (
	"github.com/google/uuid"
)

type State interface {
	isPlayerState()
}

type UnregisteredPlayer struct{}

func (UnregisteredPlayer) isPlayerState() {}

// 登録した日時・Issuer・Subject は持たない（Decide もコマンドも読まず、一意性は一意な key が守るため）
type Player struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
}

func (Player) isPlayerState() {}

func InitialState() State {
	return UnregisteredPlayer{}
}
