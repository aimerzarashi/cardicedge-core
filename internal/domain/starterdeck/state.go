package starterdeck

import (
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type State interface {
	isStarterDeckState()
}

type UnregisteredStarterDeck struct{}

func (UnregisteredStarterDeck) isStarterDeckState() {}

type StarterDeck struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	Name     Name
	// 枚数が 0 のカードは持たない
	Cards        map[CardRef]CardCount
	RegisteredBy operator.ID
	RegisteredAt time.Time
	UpdatedAt    time.Time
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
