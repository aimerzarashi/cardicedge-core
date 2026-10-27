package studiorules

import "github.com/google/uuid"

type State interface {
	isStudioRulesState()
}

type UncreatedStudioRules struct{}

func (UncreatedStudioRules) isStudioRulesState() {}

type StudioRules struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	Rules    Rules
}

func (StudioRules) isStudioRulesState() {}

func InitialState() State {
	return UncreatedStudioRules{}
}
