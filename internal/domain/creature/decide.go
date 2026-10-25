package creature

import "time"

// 変わらない操作（公開中にもう一度公開する等）は、イベントを作らない（版を進めず、画面の版を古くしないため）
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredCreature); !ok {
			return nil, ErrInvalidState
		}
		return []Event{&RegisteredEvent{
			EventMeta:    EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:     c.PublicID,
			Name:         c.Name,
			RegisteredBy: c.RegisteredBy,
		}}, nil
	case *PublishCommand:
		creature, ok := state.(Creature)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := creature.Status.Publish()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&PublishedEvent{EventMeta: nextMeta(creature, now), PublishedBy: c.PublishedBy}}, nil
	case *RetireCommand:
		creature, ok := state.(Creature)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := creature.Status.Retire()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&RetiredEvent{EventMeta: nextMeta(creature, now), RetiredBy: c.RetiredBy}}, nil
	default:
		return nil, ErrInvalidState
	}
}

func nextMeta(creature Creature, now time.Time) EventMeta {
	return EventMeta{EntityID: creature.ID, AggregateVersion: creature.Version + 1, OccurredAt: now}
}
