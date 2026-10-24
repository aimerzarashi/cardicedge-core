package player

import "time"

// 中で time.Now を呼ばず発生時刻を受け取るのは、テストで時刻を固定するため（設計メモ 6 節の 5）
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredPlayer); !ok {
			return nil, ErrAlreadyRegistered
		}
		return []Event{&RegisteredEvent{
			EventMeta: EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:  c.PublicID,
			Issuer:    c.Issuer,
			Subject:   c.Subject,
		}}, nil
	default:
		return nil, ErrInvalidState
	}
}
