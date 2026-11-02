package presetdeck

import "maps"

// 状態を書き換えないよう、カードの一覧は写してから変える
func Evolve(state State, event Event) (State, error) {
	meta := event.GetMeta()
	switch e := event.(type) {
	case *RegisteredEvent:
		if _, ok := state.(UnregisteredPresetDeck); !ok {
			return state, ErrInvalidState
		}
		return PresetDeck{
			ID:       meta.EntityID,
			Version:  meta.AggregateVersion,
			PublicID: e.PublicID,
			Name:     e.Name,
			Cards:    map[CardRef]CardCount{},
		}, nil
	case *RenamedEvent:
		deck, ok := state.(PresetDeck)
		if !ok {
			return state, ErrInvalidState
		}
		deck.Name = e.Name
		return touched(deck, meta), nil
	case *CardCountsChangedEvent:
		deck, ok := state.(PresetDeck)
		if !ok {
			return state, ErrInvalidState
		}
		cards := maps.Clone(deck.Cards)
		for _, change := range e.Changes {
			count, err := NewCardCount(cards[change.Card].Int() + change.Delta)
			if err != nil {
				return state, err
			}
			if count == 0 {
				delete(cards, change.Card)
			} else {
				cards[change.Card] = count
			}
		}
		deck.Cards = cards
		return touched(deck, meta), nil
	case *DeletedEvent:
		deck, ok := state.(PresetDeck)
		if !ok {
			return state, ErrInvalidState
		}
		return DeletedPresetDeck{ID: deck.ID, Version: meta.AggregateVersion}, nil
	default:
		return state, ErrInvalidState
	}
}

func touched(deck PresetDeck, meta EventMeta) PresetDeck {
	deck.Version = meta.AggregateVersion
	return deck
}
