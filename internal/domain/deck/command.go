package deck

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
)

type Command interface {
	isDeckCommand()
}

type CreateCommand struct {
	ID           ID
	Player       player.ID
	Name         Name
	Cards        []startergrant.CardCount
	StarterGrant startergrant.ID
	Number       startergrant.DeckNumber
}

func (*CreateCommand) isDeckCommand() {}
