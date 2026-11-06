package presetdeck

import (
	"maps"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

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
			Cards:    map[card.Ref]card.DeckQuantity{},
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
			quantity, remains, err := cards[change.Card].Add(change.Delta)
			if err != nil {
				return state, err
			}
			if remains {
				cards[change.Card] = quantity
			} else {
				delete(cards, change.Card)
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
