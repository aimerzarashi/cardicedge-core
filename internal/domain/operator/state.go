package operator

import (
	"time"

	"github.com/google/uuid"
)

type State interface {
	isOperatorState()
}

type UnregisteredOperator struct{}

func (UnregisteredOperator) isOperatorState() {}

// Issuer・Subject・LinkedAt は、紐づけるまでゼロ値
type Operator struct {
	ID           ID
	Version      int64
	PublicID     uuid.UUID
	Email        Email
	RegisteredAt time.Time
	Issuer       Issuer
	Subject      Subject
	LinkedAt     time.Time
}

func (Operator) isOperatorState() {}

func (o Operator) Linked() bool { return o.Subject != "" }

func InitialState() State {
	return UnregisteredOperator{}
}
