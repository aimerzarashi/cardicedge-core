package board

import (
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type State interface {
	isBoardState()
}

type UnregisteredBoard struct{}

func (UnregisteredBoard) isBoardState() {}

type Board struct {
	ID           ID
	Version      int64
	PublicID     uuid.UUID
	Name         Name
	Status       Status
	Layout       Layout
	DefaultRules DefaultRules
	RegisteredBy operator.ID
	RegisteredAt time.Time
	UpdatedAt    time.Time
}

func (Board) isBoardState() {}

// 削除した後も、版を持つ（削除の後の操作を、版の衝突ではなく状態の誤りにするため）
type DeletedBoard struct {
	ID      ID
	Version int64
}

func (DeletedBoard) isBoardState() {}

func InitialState() State {
	return UnregisteredBoard{}
}
