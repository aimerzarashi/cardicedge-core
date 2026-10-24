package deck

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
)

type State interface {
	isDeckState()
}

type UncreatedDeck struct{}

func (UncreatedDeck) isDeckState() {}

type Deck struct {
	ID      ID
	Version int64
	Owner   player.ID
	Name    Name
	Cards   []startergrant.CardCount
	Grant   startergrant.ID
	Number  startergrant.DeckNumber
}

func (Deck) isDeckState() {}

func InitialState() State {
	return UncreatedDeck{}
}
