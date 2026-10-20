package starterdeckset

import (
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

type State interface {
	isStarterDeckSetState()
}

// まだ一度も設定していない（集約にイベントが無い）。版は 0
type UnsetStarterDeckSet struct{}

func (UnsetStarterDeckSet) isStarterDeckSetState() {}

type StarterDeckSet struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	// 元のプリセットデッキの公開 ID の順に並べる（同じ組を、同じ値として比べるため）
	Decks []Deck
	SetBy operator.ID
	SetAt time.Time
}

func (StarterDeckSet) isStarterDeckSetState() {}

func InitialState() State {
	return UnsetStarterDeckSet{}
}

// 写したすべてのカード（参照をそろえるのに使う）。同じカードが 2 つのスターターデッキに入っていても 1 つにする
func (s StarterDeckSet) Cards() []presetdeck.CardRef {
	seen := map[presetdeck.CardRef]bool{}
	var cards []presetdeck.CardRef
	for _, deck := range s.Decks {
		for _, c := range deck.cards {
			if !seen[c.card] {
				seen[c.card] = true
				cards = append(cards, c.card)
			}
		}
	}
	return cards
}
