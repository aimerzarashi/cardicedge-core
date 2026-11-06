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
	Cards        []Card
	StarterGrant startergrant.ID
}

func (*CreateCommand) isCollectionCommand() {}

// カードを所持品に足す。足す理由は出どころで表す
type AcquireCommand struct {
	Cards  []Card
	Source Source
}

func (*AcquireCommand) isCollectionCommand() {}
