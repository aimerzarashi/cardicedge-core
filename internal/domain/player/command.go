package player

import "github.com/google/uuid"

type Command interface {
	isPlayerCommand()
}

type RegisterCommand struct {
	ID       ID
	PublicID uuid.UUID
	Issuer   Issuer
	Subject  Subject
}

func (*RegisterCommand) isPlayerCommand() {}
