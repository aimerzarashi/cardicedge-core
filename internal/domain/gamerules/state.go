package gamerules

import "github.com/google/uuid"

type State interface {
	isGameRulesState()
}

type UncreatedGameRules struct{}

func (UncreatedGameRules) isGameRulesState() {}

type GameRules struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	Rules    Rules
}

func (GameRules) isGameRulesState() {}

func InitialState() State {
	return UncreatedGameRules{}
}
