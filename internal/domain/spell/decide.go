package spell

import "time"

// 変わらない操作（公開中にもう一度公開する等）は、イベントを作らない（版を進めず、画面の版を古くしないため）
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredSpell); !ok {
			return nil, ErrInvalidState
		}
		return []Event{&RegisteredEvent{
			EventMeta:    EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:     c.PublicID,
			Name:         c.Name,
			RegisteredBy: c.RegisteredBy,
		}}, nil
	case *PublishCommand:
		spell, ok := state.(Spell)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := spell.Status.Publish()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&PublishedEvent{EventMeta: nextMeta(spell, now), PublishedBy: c.PublishedBy}}, nil
	case *RetireCommand:
		spell, ok := state.(Spell)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := spell.Status.Retire()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&RetiredEvent{EventMeta: nextMeta(spell, now), RetiredBy: c.RetiredBy}}, nil
	default:
		return nil, ErrInvalidState
	}
}

func nextMeta(spell Spell, now time.Time) EventMeta {
	return EventMeta{EntityID: spell.ID, AggregateVersion: spell.Version + 1, OccurredAt: now}
}
