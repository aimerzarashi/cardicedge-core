package creature

func Evolve(state State, event Event) (State, error) {
	switch e := event.(type) {
	case *RegisteredEvent:
		if _, ok := state.(UnregisteredCreature); !ok {
			return state, ErrInvalidState
		}
		meta := e.GetMeta()
		return Creature{
			ID:           meta.EntityID,
			Version:      meta.AggregateVersion,
			PublicID:     e.PublicID,
			Name:         e.Name,
			RegisteredBy: e.RegisteredBy,
			RegisteredAt: meta.OccurredAt,
		}, nil
	default:
		return state, ErrInvalidState
	}
}
