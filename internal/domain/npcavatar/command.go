package npcavatar

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type Command interface {
	isNPCAvatarCommand()
}

type RegisterCommand struct {
	ID           ID
	PublicID     uuid.UUID
	Name         Name
	RegisteredBy operator.ID
}

type RenameCommand struct {
	Name      Name
	RenamedBy operator.ID
}

type PublishCommand struct {
	PublishedBy operator.ID
}

type RetireCommand struct {
	RetiredBy operator.ID
}

type DeleteCommand struct {
	DeletedBy operator.ID
}

func (*RegisterCommand) isNPCAvatarCommand() {}
func (*RenameCommand) isNPCAvatarCommand()   {}
func (*PublishCommand) isNPCAvatarCommand()  {}
func (*RetireCommand) isNPCAvatarCommand()   {}
func (*DeleteCommand) isNPCAvatarCommand()   {}
