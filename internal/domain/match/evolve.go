package match

import "slices"

// 状態は値で返し、渡された状態を書き換えない
func Evolve(state State, event Event) (State, error) {
	meta := event.GetMeta()
	if created, ok := event.(*CreatedEvent); ok {
		if _, uncreated := state.(UncreatedMatch); !uncreated {
			return state, ErrInvalidState
		}
		base, _ := created.Board.base()
		participants := make([]ParticipantState, len(created.Participants))
		for i, participant := range created.Participants {
			participants[i] = ParticipantState{Participant: participant, Position: base, Magic: created.Rules.StartingMagic}
		}
		return Match{
			ID: meta.EntityID, Version: meta.AggregateVersion, Board: created.Board, Rules: created.Rules,
			Participants: participants, Round: 1,
		}, nil
	}
	current, ok := state.(Match)
	if !ok {
		return state, ErrInvalidState
	}
	current.Version = meta.AggregateVersion
	switch e := event.(type) {
	case *TurnOrderDecidedEvent:
		current.TurnOrder = e.Order
		current.Turn = 0
	case *DecksShuffledEvent:
		if len(e.DrawPiles) != len(current.Participants) {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		for i := range current.Participants {
			current.Participants[i].DrawPile = e.DrawPiles[i]
		}
	case *HandsDealtEvent:
		current.Participants = slices.Clone(current.Participants)
		for i, participant := range current.Participants {
			count := min(e.Cards, len(participant.DrawPile))
			current.Participants[i].Hand = append(slices.Clone(participant.Hand), participant.DrawPile[:count]...)
			current.Participants[i].DrawPile = participant.DrawPile[count:]
		}
	default:
		return state, ErrInvalidState
	}
	return current, nil
}
