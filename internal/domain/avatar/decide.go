package avatar

import "time"

// 中で time.Now を呼ばず発生時刻を受け取るのは、テストで時刻を固定するため
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredAvatar); !ok {
			return nil, ErrAlreadyRegistered
		}
		return []Event{&RegisteredEvent{
			EventMeta: EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:  c.PublicID,
			Owner:     c.Owner,
			Name:      c.Name,
		}}, nil
	default:
		return nil, ErrInvalidState
	}
}
