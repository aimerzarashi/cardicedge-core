package collection

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
)

type Command interface {
	isCollectionCommand()
}

type CreateCommand struct {
	ID           ID
	Player       player.ID
	Cards        []startergrant.CardCount
	StarterGrant startergrant.ID
}

func (*CreateCommand) isCollectionCommand() {}
