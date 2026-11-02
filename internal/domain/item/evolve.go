package item

import "gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"

func Evolve(state State, event Event) (State, error) {
	switch e := event.(type) {
	case *RegisteredEvent:
		if _, ok := state.(UnregisteredItem); !ok {
			return state, ErrInvalidState
		}
		meta := e.GetMeta()
		return Item{
			ID:       meta.EntityID,
			Version:  meta.AggregateVersion,
			PublicID: e.PublicID,
			Status:   card.StatusDraft,
		}, nil
	case *PublishedEvent:
		item, ok := state.(Item)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := item.Status.Publish()
		if err != nil {
			return state, err
		}
		item.Status = status
		item.Version = e.GetMeta().AggregateVersion
		return item, nil
	case *RetiredEvent:
		item, ok := state.(Item)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := item.Status.Retire()
		if err != nil {
			return state, err
		}
		item.Status = status
		item.Version = e.GetMeta().AggregateVersion
		return item, nil
	default:
		return state, ErrInvalidState
	}
}
