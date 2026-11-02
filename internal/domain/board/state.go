package board

import (
	"github.com/google/uuid"
)

type State interface {
	isBoardState()
}

type UnregisteredBoard struct{}

func (UnregisteredBoard) isBoardState() {}

// 登録した人・日時は持たない（Decide が使わず、画面に出す値は read model がイベントから作るため）
type Board struct {
	ID           ID
	Version      int64
	PublicID     uuid.UUID
	Name         Name
	Status       Status
	Layout       Layout
	DefaultRules DefaultRules
}

func (Board) isBoardState() {}

// 削除した後も、版を持つ（削除の後の操作を、版の衝突ではなく状態の誤りにするため）
type DeletedBoard struct {
	ID      ID
	Version int64
}

func (DeletedBoard) isBoardState() {}

func InitialState() State {
	return UnregisteredBoard{}
}
