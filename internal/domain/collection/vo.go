package collection

import (
	"errors"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

type ID int64

var entityType = domain.NewEntityType("collection")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("collection id must be positive")
	}
	return ID(value), nil
}

// 所持するカードと枚数。カードは game の内部 ID で指す。配布の中身（startergrant.CollectionCard）の型を使わないのは、
// 長く残る所持品が 1 回で終わる配布の流れに依存しないため
type Card struct {
	Card     card.Ref           `json:"card"`
	Quantity card.OwnedQuantity `json:"quantity"`
}
