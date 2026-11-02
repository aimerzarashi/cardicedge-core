package avatar

// 状態は値で返し、渡された状態を書き換えない
func Evolve(state State, event Event) (State, error) {
	switch e := event.(type) {
	case *RegisteredEvent:
		if _, ok := state.(UnregisteredAvatar); !ok {
			return state, ErrInvalidState
		}
		meta := e.GetMeta()
		return Avatar{ID: meta.EntityID, Version: meta.AggregateVersion, PublicID: e.PublicID}, nil
	default:
		return state, ErrInvalidState
	}
}
