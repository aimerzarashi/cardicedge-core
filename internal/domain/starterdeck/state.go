package starterdeck

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

type State interface {
	isStarterDeckState()
}

type UnregisteredStarterDeck struct{}

func (UnregisteredStarterDeck) isStarterDeckState() {}

// 登録した人・日時・写し元は持たない（Decide もコマンドも読まず、画面に出す値は read model がイベントから作るため）
type StarterDeck struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	Name     presetdeck.Name
	Status   Status
	// 枚数が 0 のカードは持たない
	Cards map[presetdeck.CardRef]presetdeck.CardCount
}

func (StarterDeck) isStarterDeckState() {}

func (d StarterDeck) TotalCount() int {
	total := 0
	for _, count := range d.Cards {
		total += count.Int()
	}
	return total
}

// 削除した後も、版を持つ（削除の後の操作を、版の衝突ではなく状態の誤りにするため）
type DeletedStarterDeck struct {
	ID      ID
	Version int64
}

func (DeletedStarterDeck) isStarterDeckState() {}

func InitialState() State {
	return UnregisteredStarterDeck{}
}
