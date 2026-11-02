package operator

import (
	"github.com/google/uuid"
)

type State interface {
	isOperatorState()
}

type UnregisteredOperator struct{}

func (UnregisteredOperator) isOperatorState() {}

// 登録・紐づけた日時・Email・Issuer は持たない（Decide もコマンドも読まず、一意性は一意な key が守るため）。Subject だけ、紐づけ済みか（Linked）の判定に持ち、紐づけるまでゼロ値
type Operator struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	Subject  Subject
}

func (Operator) isOperatorState() {}

func (o Operator) Linked() bool {
	return o.Subject != ""
}

func InitialState() State {
	return UnregisteredOperator{}
}
