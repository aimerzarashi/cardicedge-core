package startergrant

import "slices"

// 状態は値で返し、渡された状態を書き換えない（DeckRecorded は写してから変える）
func Evolve(state State, event Event) (State, error) {
	meta := event.GetMeta()
	switch e := event.(type) {
	case *StartedEvent:
		if _, ok := state.(NotStartedStarterGrant); !ok {
			return state, ErrInvalidState
		}
		return InProgressStarterGrant{ID: meta.EntityID, Version: meta.AggregateVersion, Player: e.Player, Cards: e.Cards, Decks: e.Decks, DeckRecorded: make([]bool, len(e.Decks))}, nil
	case *CollectionRecordedEvent:
		progress, ok := state.(InProgressStarterGrant)
		if !ok {
			return state, ErrInvalidState
		}
		progress.Version = meta.AggregateVersion
		progress.CollectionRecorded = true
		return progress, nil
	case *DeckRecordedEvent:
		progress, ok := state.(InProgressStarterGrant)
		if !ok {
			return state, ErrInvalidState
		}
		progress.Version = meta.AggregateVersion
		if e.Number.Int() > len(progress.DeckRecorded) {
			return state, ErrInvalidDeckNumber
		}
		recorded := slices.Clone(progress.DeckRecorded)
		recorded[e.Number.Int()-1] = true
		progress.DeckRecorded = recorded
		return progress, nil
	case *CompletedEvent:
		progress, ok := state.(InProgressStarterGrant)
		if !ok {
			return state, ErrInvalidState
		}
		return CompletedStarterGrant{ID: progress.ID, Version: meta.AggregateVersion, Player: progress.Player}, nil
	default:
		return state, ErrInvalidState
	}
}
