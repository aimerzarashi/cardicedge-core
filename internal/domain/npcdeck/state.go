package npcdeck

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

type State interface {
	isNPCDeckState()
}

type UnregisteredNPCDeck struct{}

func (UnregisteredNPCDeck) isNPCDeckState() {}

// 登録した人・日時・写し元は持たない（Decide もコマンドも読まず、画面に出す値は read model がイベントから作るため）
type NPCDeck struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	Name     presetdeck.Name
	Status   Status
	// 枚数が 0 のカードは持たない（DeckQuantity は 1 以上）
	Cards map[card.Ref]card.DeckQuantity
}

func (NPCDeck) isNPCDeckState() {}

func (d NPCDeck) TotalCount() int {
	total := 0
	for _, count := range d.Cards {
		total += count.Int()
	}
	return total
}

// 削除した後も、版を持つ（削除の後の操作を、版の衝突ではなく状態の誤りにするため）
type DeletedNPCDeck struct {
	ID      ID
	Version int64
}

func (DeletedNPCDeck) isNPCDeckState() {}

func InitialState() State {
	return UnregisteredNPCDeck{}
}
