package operator

import "github.com/google/uuid"

type Command interface {
	isOperatorCommand()
}

type RegisterCommand struct {
	ID       ID
	PublicID uuid.UUID
	Email    Email
}

func (*RegisterCommand) isOperatorCommand() {}

type LinkCommand struct {
	Issuer  Issuer
	Subject Subject
}

func (*LinkCommand) isOperatorCommand() {}
