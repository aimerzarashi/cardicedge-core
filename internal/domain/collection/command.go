package collection

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
)

type Command interface {
	isCollectionCommand()
}

type CreateCommand struct {
	ID    ID
	Owner player.ID
	Cards []startergrant.CardCount
	Grant startergrant.ID
}

func (*CreateCommand) isCollectionCommand() {}
