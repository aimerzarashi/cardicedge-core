package collection

import "slices"

// 状態は値で返し、渡された状態を書き換えない
func Evolve(state State, event Event) (State, error) {
	switch e := event.(type) {
	case *CreatedEvent:
		if _, ok := state.(UncreatedCollection); !ok {
			return state, ErrInvalidState
		}
		meta := e.GetMeta()
		return Collection{ID: meta.EntityID, Version: meta.AggregateVersion}, nil
	case *CardsAcquiredEvent:
		current, ok := state.(Collection)
		if !ok {
			return state, ErrInvalidState
		}
		current.Version = e.GetMeta().AggregateVersion
		if e.Source.MatchReward != nil {
			current.RewardedMatches = append(slices.Clone(current.RewardedMatches), e.Source.MatchReward.Match)
		}
		return current, nil
	default:
		return state, ErrInvalidState
	}
}
