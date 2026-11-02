package item

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

type State interface {
	isItemState()
}

type UnregisteredItem struct{}

func (UnregisteredItem) isItemState() {}

// 登録した人・日時・名前は持たない（Decide もコマンドも読まず、画面に出す値は read model がイベントから作るため）
type Item struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	Status   card.Status
}

func (Item) isItemState() {}

func InitialState() State {
	return UnregisteredItem{}
}
