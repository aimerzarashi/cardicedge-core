package operator

// 状態は値で返し、渡された状態を書き換えない（参照の api はポインタの状態を書き換えていた。設計メモ 6 節の 5）
func Evolve(state State, event Event) (State, error) {
	switch e := event.(type) {
	case *RegisteredEvent:
		if _, ok := state.(UnregisteredOperator); !ok {
			return state, ErrInvalidState
		}
		meta := e.GetMeta()
		return Operator{
			ID:           meta.EntityID,
			Version:      meta.AggregateVersion,
			PublicID:     e.PublicID,
			Email:        e.Email,
			RegisteredAt: meta.OccurredAt,
		}, nil
	case *LinkedEvent:
		s, ok := state.(Operator)
		if !ok || s.Linked() {
			return state, ErrInvalidState
		}
		meta := e.GetMeta()
		s.Version = meta.AggregateVersion
		s.Issuer = e.Issuer
		s.Subject = e.Subject
		s.LinkedAt = meta.OccurredAt
		return s, nil
	default:
		return state, ErrInvalidState
	}
}
