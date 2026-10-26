package gamerules

func Evolve(state State, event Event) (State, error) {
	switch e := event.(type) {
	case *CreatedEvent:
		if _, ok := state.(UncreatedGameRules); !ok {
			return state, ErrInvalidState
		}
		meta := e.GetMeta()
		return GameRules{ID: meta.EntityID, Version: meta.AggregateVersion, PublicID: e.PublicID, Rules: e.Rules}, nil
	default:
		return state, ErrInvalidState
	}
}
