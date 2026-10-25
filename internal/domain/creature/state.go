package creature

import (
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type State interface {
	isCreatureState()
}

type UnregisteredCreature struct{}

func (UnregisteredCreature) isCreatureState() {}

type Creature struct {
	ID           ID
	Version      int64
	PublicID     uuid.UUID
	Name         card.Name
	Status       card.Status
	RegisteredBy operator.ID
	RegisteredAt time.Time
}

func (Creature) isCreatureState() {}

func InitialState() State {
	return UnregisteredCreature{}
}
