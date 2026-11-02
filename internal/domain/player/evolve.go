package player

// 状態は値で返し、渡された状態を書き換えない（参照の api はポインタの状態を書き換えていた。設計メモ 6 節の 5）
func Evolve(state State, event Event) (State, error) {
	switch e := event.(type) {
	case *RegisteredEvent:
		if _, ok := state.(UnregisteredPlayer); !ok {
			return state, ErrInvalidState
		}
		meta := e.GetMeta()
		return Player{
			ID:       meta.EntityID,
			Version:  meta.AggregateVersion,
			PublicID: e.PublicID,
		}, nil
	default:
		return state, ErrInvalidState
	}
}
