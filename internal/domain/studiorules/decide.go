package studiorules

import "time"

func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *CreateCommand:
		if _, ok := state.(UncreatedStudioRules); !ok {
			return nil, ErrAlreadyCreated
		}
		return []Event{&CreatedEvent{
			EventMeta: EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:  c.PublicID,
			Rules:     c.Rules,
		}}, nil
	default:
		return nil, ErrInvalidState
	}
}
