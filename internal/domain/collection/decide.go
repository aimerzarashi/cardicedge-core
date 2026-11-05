package collection

import (
	"slices"
	"time"
)

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
	case *AcquireCommand:
		current, ok := state.(Collection)
		if !ok {
			return nil, ErrInvalidState
		}
		if len(c.Cards) == 0 || c.Source.MatchReward == nil {
			return nil, ErrInvalidAcquisition
		}
		if slices.Contains(current.RewardedMatches, c.Source.MatchReward.Match) {
			return nil, ErrAlreadyAcquired
		}
		return []Event{&CardsAcquiredEvent{
			EventMeta: EventMeta{EntityID: current.ID, AggregateVersion: current.Version + 1, OccurredAt: now},
			Cards:     c.Cards,
			Source:    c.Source,
		}}, nil
	default:
		return nil, ErrInvalidState
	}
}
