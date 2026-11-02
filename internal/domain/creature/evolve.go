package creature

import "gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"

func Evolve(state State, event Event) (State, error) {
	switch e := event.(type) {
	case *RegisteredEvent:
		if _, ok := state.(UnregisteredCreature); !ok {
			return state, ErrInvalidState
		}
		meta := e.GetMeta()
		return Creature{
			ID:       meta.EntityID,
			Version:  meta.AggregateVersion,
			PublicID: e.PublicID,
			Status:   card.StatusDraft,
		}, nil
	case *PublishedEvent:
		creature, ok := state.(Creature)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := creature.Status.Publish()
		if err != nil {
			return state, err
		}
		creature.Status = status
		creature.Version = e.GetMeta().AggregateVersion
		return creature, nil
	case *RetiredEvent:
		creature, ok := state.(Creature)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := creature.Status.Retire()
		if err != nil {
			return state, err
		}
		creature.Status = status
		creature.Version = e.GetMeta().AggregateVersion
		return creature, nil
	default:
		return state, ErrInvalidState
	}
}
