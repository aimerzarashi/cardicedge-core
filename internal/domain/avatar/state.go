package avatar

import (
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

type State interface {
	isAvatarState()
}

type UnregisteredAvatar struct{}

func (UnregisteredAvatar) isAvatarState() {}

type Avatar struct {
	ID           ID
	Version      int64
	PublicID     uuid.UUID
	Owner        player.ID
	Name         Name
	RegisteredAt time.Time
}

func (Avatar) isAvatarState() {}

func InitialState() State {
	return UnregisteredAvatar{}
}
