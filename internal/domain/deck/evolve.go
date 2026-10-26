package deck

// 状態は値で返し、渡された状態を書き換えない
func Evolve(state State, event Event) (State, error) {
	switch e := event.(type) {
	case *CreatedEvent:
		if _, ok := state.(UncreatedDeck); !ok {
			return state, ErrInvalidState
		}
		meta := e.GetMeta()
		return Deck{ID: meta.EntityID, Version: meta.AggregateVersion, Player: e.Player, Name: e.Name, Cards: e.Cards, StarterGrant: e.StarterGrant, Number: e.Number}, nil
	default:
		return state, ErrInvalidState
	}
}
