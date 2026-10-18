package spell

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type Command interface {
	isSpellCommand()
}

type RegisterCommand struct {
	ID           ID
	PublicID     uuid.UUID
	Name         card.Name
	RegisteredBy operator.ID
}

func (*RegisterCommand) isSpellCommand() {}
