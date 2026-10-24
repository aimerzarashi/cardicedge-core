package deck

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
)

type Command interface {
	isDeckCommand()
}

type CreateCommand struct {
	ID     ID
	Owner  player.ID
	Name   Name
	Cards  []startergrant.CardCount
	Grant  startergrant.ID
	Number startergrant.DeckNumber
}

func (*CreateCommand) isDeckCommand() {}
