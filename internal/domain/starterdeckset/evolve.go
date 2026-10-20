package starterdeckset

func Evolve(state State, event Event) (State, error) {
	meta := event.GetMeta()
	switch e := event.(type) {
	case *DecksSetEvent:
		switch state.(type) {
		case UnsetStarterDeckSet, StarterDeckSet:
		default:
			return state, ErrInvalidState
		}
		return StarterDeckSet{
			ID:       meta.EntityID,
			Version:  meta.AggregateVersion,
			PublicID: e.PublicID,
			Decks:    e.Decks,
			SetBy:    e.SetBy,
			SetAt:    meta.OccurredAt,
		}, nil
	default:
		return state, ErrInvalidState
	}
}
