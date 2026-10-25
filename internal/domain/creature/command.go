package creature

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type Command interface {
	isCreatureCommand()
}

type RegisterCommand struct {
	ID           ID
	PublicID     uuid.UUID
	Name         card.Name
	RegisteredBy operator.ID
}

// 公開中にする（準備中・使用不可から）。公開中なら何も変えない
type PublishCommand struct {
	PublishedBy operator.ID
}

// 使用不可にする（公開中から）。使用不可なら何も変えない。準備中からはできない
type RetireCommand struct {
	RetiredBy operator.ID
}

func (*RegisterCommand) isCreatureCommand() {}
func (*PublishCommand) isCreatureCommand()  {}
func (*RetireCommand) isCreatureCommand()   {}
