package collection

import "time"

// 中で time.Now を呼ばず発生時刻を受け取るのは、テストで時刻を固定するため
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *CreateCommand:
		if _, ok := state.(UncreatedCollection); !ok {
			return nil, ErrAlreadyCreated
		}
		return []Event{&CreatedEvent{
			EventMeta:    EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			Player:       c.Player,
			Cards:        c.Cards,
			StarterGrant: c.StarterGrant,
		}}, nil
	default:
		return nil, ErrInvalidState
	}
}
