package room

import "slices"

// 状態は値で返し、渡された状態を書き換えない
func Evolve(state State, event Event) (State, error) {
	meta := event.GetMeta()
	if created, ok := event.(*CreatedEvent); ok {
		if _, uncreated := state.(UncreatedRoom); !uncreated {
			return state, ErrInvalidState
		}
		return Room{ID: meta.EntityID, Version: meta.AggregateVersion, Owner: created.Player, Name: created.Name, Status: StatusDraft}, nil
	}
	current, ok := state.(Room)
	if !ok {
		return state, ErrInvalidState
	}
	current.Version = meta.AggregateVersion
	switch e := event.(type) {
	case *SeatsChangedEvent:
		current.Seats = make([]SeatState, len(e.Seats))
		for i, seat := range e.Seats {
			current.Seats[i] = SeatState{Seat: seat}
		}
	case *MatchRulesChangedEvent:
		rules := e.MatchRules
		current.MatchRules = &rules
	case *SeatChoiceChangedEvent:
		if e.Seat.Int() >= len(current.Seats) {
			return state, ErrInvalidState
		}
		current.Seats = slices.Clone(current.Seats)
		choice := e.Choice
		current.Seats[e.Seat].Choice = &choice
	case *PublishedEvent:
		// この時点で決まっていたものは、準備中に変えられない
		current.Seats = slices.Clone(current.Seats)
		for i := range current.Seats {
			current.Seats[i].ChoiceLocked = current.Seats[i].Choice != nil
		}
		current.MatchRulesLocked = current.MatchRules != nil
	case *PreparationStartedEvent:
		current.Status = StatusPreparing
	case *ReadyDeclaredEvent:
		current.Ready = append(slices.Clone(current.Ready), e.Player)
	case *ClosedEvent:
		current.Status = StatusClosed
	case *RenamedEvent:
		current.Name = e.Name
	default:
		return state, ErrInvalidState
	}
	return current, nil
}
