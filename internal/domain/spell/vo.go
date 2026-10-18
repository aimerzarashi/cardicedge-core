package spell

import (
	"errors"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

type ID int64

var entityType = domain.NewEntityType("spell")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("spell id must be positive")
	}
	return ID(value), nil
}
