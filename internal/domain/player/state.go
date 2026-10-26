package player

import (
	"time"

	"github.com/google/uuid"
)

type State interface {
	isPlayerState()
}

type UnregisteredPlayer struct{}

func (UnregisteredPlayer) isPlayerState() {}

type Player struct {
	ID           ID
	Version      int64
	PublicID     uuid.UUID
	Issuer       Issuer
	Subject      Subject
	RegisteredAt time.Time
}

func (Player) isPlayerState() {}

func InitialState() State {
	return UnregisteredPlayer{}
}
