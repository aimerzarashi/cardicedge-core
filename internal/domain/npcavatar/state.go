package npcavatar

import (
	"github.com/google/uuid"
)

type State interface {
	isNPCAvatarState()
}

type UnregisteredNPCAvatar struct{}

func (UnregisteredNPCAvatar) isNPCAvatarState() {}

// 登録した人・日時は持たない（Decide が使わず、画面に出す値は read model がイベントから作るため）
type NPCAvatar struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	Name     Name
	Status   Status
}

func (NPCAvatar) isNPCAvatarState() {}

// 削除した後も、版を持つ（削除の後の操作を、版の衝突ではなく状態の誤りにするため）
type DeletedNPCAvatar struct {
	ID      ID
	Version int64
}

func (DeletedNPCAvatar) isNPCAvatarState() {}

func InitialState() State {
	return UnregisteredNPCAvatar{}
}
