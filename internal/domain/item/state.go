package item

import (
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type State interface {
	isItemState()
}

type UnregisteredItem struct{}

func (UnregisteredItem) isItemState() {}

type Item struct {
	ID           ID
	Version      int64
	PublicID     uuid.UUID
	Name         card.Name
	Status       card.Status
	RegisteredBy operator.ID
	RegisteredAt time.Time
}

func (Item) isItemState() {}

func InitialState() State {
	return UnregisteredItem{}
}
