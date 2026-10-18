package spell

import (
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type State interface {
	isSpellState()
}

type UnregisteredSpell struct{}

func (UnregisteredSpell) isSpellState() {}

type Spell struct {
	ID           ID
	Version      int64
	PublicID     uuid.UUID
	Name         card.Name
	RegisteredBy operator.ID
	RegisteredAt time.Time
}

func (Spell) isSpellState() {}

func InitialState() State {
	return UnregisteredSpell{}
}
