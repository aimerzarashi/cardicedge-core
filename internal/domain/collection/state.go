package collection

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
)

type State interface {
	isCollectionState()
}

type UncreatedCollection struct{}

func (UncreatedCollection) isCollectionState() {}

type Collection struct {
	ID      ID
	Version int64
	Owner   player.ID
	Cards   []startergrant.CardCount
	Grant   startergrant.ID
}

func (Collection) isCollectionState() {}

func InitialState() State {
	return UncreatedCollection{}
}
