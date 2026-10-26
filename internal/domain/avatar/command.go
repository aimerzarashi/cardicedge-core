package avatar

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

type Command interface {
	isAvatarCommand()
}

type RegisterCommand struct {
	ID       ID
	PublicID uuid.UUID
	Player   player.ID
	Name     Name
}

func (*RegisterCommand) isAvatarCommand() {}
