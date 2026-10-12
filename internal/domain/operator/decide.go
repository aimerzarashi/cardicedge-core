package operator

import "time"

// 中で time.Now を呼ばず発生時刻を受け取るのは、テストで時刻を固定するため（設計メモ 6 節の 5）
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredOperator); !ok {
			return nil, ErrInvalidState
		}
		return []Event{&RegisteredEvent{
			EventMeta: EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:  c.PublicID,
			Email:     c.Email,
		}}, nil
	case *LinkCommand:
		s, ok := state.(Operator)
		if !ok {
			return nil, ErrInvalidState
		}
		// 紐づけ直しを許すと、同じメールアドレスの別のアカウントでオペレーターを乗っ取れるため
		if s.Linked() {
			return nil, ErrAlreadyLinked
		}
		return []Event{&LinkedEvent{
			EventMeta: EventMeta{EntityID: s.ID, AggregateVersion: s.Version + 1, OccurredAt: now},
			Issuer:    c.Issuer,
			Subject:   c.Subject,
		}}, nil
	default:
		return nil, ErrInvalidState
	}
}
