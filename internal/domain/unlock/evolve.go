package unlock

import "slices"

// 状態は値で返し、渡された状態を書き換えない
func Evolve(state State, event Event) (State, error) {
	meta := event.GetMeta()
	switch e := event.(type) {
	case *CreatedEvent:
		if _, ok := state.(UncreatedUnlock); !ok {
			return state, ErrInvalidState
		}
		return Unlock{ID: meta.EntityID, Version: meta.AggregateVersion, Player: e.Player}, nil
	case *BoardsUnlockedEvent:
		unlock, ok := state.(Unlock)
		if !ok {
			return state, ErrInvalidState
		}
		unlock.Version = meta.AggregateVersion
		unlock.Boards = append(slices.Clone(unlock.Boards), e.Boards...)
		return unlock, nil
	default:
		return state, ErrInvalidState
	}
}
