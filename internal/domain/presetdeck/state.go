package presetdeck

import (
	"github.com/google/uuid"
)

type State interface {
	isPresetDeckState()
}

type UnregisteredPresetDeck struct{}

func (UnregisteredPresetDeck) isPresetDeckState() {}

// 登録した人・日時は持たない（Decide が使わず、画面に出す値は read model がイベントから作るため）
type PresetDeck struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	Name     Name
	// 枚数が 0 のカードは持たない
	Cards map[CardRef]CardCount
}

func (PresetDeck) isPresetDeckState() {}

func (d PresetDeck) TotalCount() int {
	total := 0
	for _, count := range d.Cards {
		total += count.Int()
	}
	return total
}

// 削除した後も、版を持つ（削除の後の操作を、版の衝突ではなく状態の誤りにするため）
type DeletedPresetDeck struct {
	ID      ID
	Version int64
}

func (DeletedPresetDeck) isPresetDeckState() {}

func InitialState() State {
	return UnregisteredPresetDeck{}
}
