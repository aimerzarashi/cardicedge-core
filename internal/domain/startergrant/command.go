package startergrant

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

type Command interface {
	isStarterGrantCommand()
}

// 配る所持カードは、デッキから Decide が計算する。解放するボードは内部 ID（board.ID）
type StartCommand struct {
	ID           ID
	Player       player.ID
	Decks        []Deck
	UnlockBoards []board.ID
	// 解放する NPC のアバター（game の内部 ID）
	UnlockNPCAvatars []npcavatar.ID
}

func (*StartCommand) isStarterGrantCommand() {}

type RecordCollectionCommand struct{}

func (*RecordCollectionCommand) isStarterGrantCommand() {}

type RecordUnlockCommand struct{}

func (*RecordUnlockCommand) isStarterGrantCommand() {}

type RecordDeckCommand struct {
	Number DeckNumber
}

func (*RecordDeckCommand) isStarterGrantCommand() {}
