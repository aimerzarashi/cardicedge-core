package item

import "time"

// 変わらない操作（公開中にもう一度公開する等）は、イベントを作らない（版を進めず、画面の版を古くしないため）
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredItem); !ok {
			return nil, ErrInvalidState
		}
		return []Event{&RegisteredEvent{
			EventMeta:    EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:     c.PublicID,
			Name:         c.Name,
			RegisteredBy: c.RegisteredBy,
		}}, nil
	case *PublishCommand:
		item, ok := state.(Item)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := item.Status.Publish()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&PublishedEvent{EventMeta: nextMeta(item, now), PublishedBy: c.PublishedBy}}, nil
	case *RetireCommand:
		item, ok := state.(Item)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := item.Status.Retire()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&RetiredEvent{EventMeta: nextMeta(item, now), RetiredBy: c.RetiredBy}}, nil
	default:
		return nil, ErrInvalidState
	}
}

func nextMeta(item Item, now time.Time) EventMeta {
	return EventMeta{EntityID: item.ID, AggregateVersion: item.Version + 1, OccurredAt: now}
}
