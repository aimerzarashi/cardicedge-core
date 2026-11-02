package avatar

import (
	"github.com/google/uuid"
)

type State interface {
	isAvatarState()
}

type UnregisteredAvatar struct{}

func (UnregisteredAvatar) isAvatarState() {}

// 登録した日時・名前・持ち主は持たない（Decide もコマンドも読まず、画面に出す値は read model がイベントから作るため）
type Avatar struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
}

func (Avatar) isAvatarState() {}

func InitialState() State {
	return UnregisteredAvatar{}
}
