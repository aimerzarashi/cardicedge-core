package startergrant

import "gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"

type Command interface {
	isStarterGrantCommand()
}

// 配る所持カードは、デッキから Decide が計算する
type StartCommand struct {
	ID     ID
	Player player.ID
	Decks  []Deck
}

func (*StartCommand) isStarterGrantCommand() {}

type RecordCollectionCommand struct{}

func (*RecordCollectionCommand) isStarterGrantCommand() {}

type RecordDeckCommand struct {
	Number DeckNumber
}

func (*RecordDeckCommand) isStarterGrantCommand() {}
