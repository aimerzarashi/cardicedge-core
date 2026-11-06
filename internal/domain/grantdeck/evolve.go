package grantdeck

import (
	"maps"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

// 状態を書き換えないよう、カードの一覧は写してから変える
func Evolve(state State, event Event) (State, error) {
	meta := event.GetMeta()
	switch e := event.(type) {
	case *RegisteredEvent:
		if _, ok := state.(UnregisteredGrantDeck); !ok {
			return state, ErrInvalidState
		}
		cards := make(map[card.Ref]card.DeckQuantity, len(e.Cards))
		for _, entry := range e.Cards {
			cards[entry.Card] = entry.Quantity
		}
		return GrantDeck{
			ID:       meta.EntityID,
			Version:  meta.AggregateVersion,
			PublicID: e.PublicID,
			Name:     e.Name,
			Status:   StatusDraft,
			Cards:    cards,
		}, nil
	case *RenamedEvent:
		deck, ok := state.(GrantDeck)
		if !ok {
			return state, ErrInvalidState
		}
		deck.Name = e.Name
		return touched(deck, meta), nil
	case *CardCountsChangedEvent:
		deck, ok := state.(GrantDeck)
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
	case *PublishedEvent:
		deck, ok := state.(GrantDeck)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := deck.Status.Publish()
		if err != nil {
			return state, err
		}
		deck.Status = status
		return touched(deck, meta), nil
	case *RetiredEvent:
		deck, ok := state.(GrantDeck)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := deck.Status.Retire()
		if err != nil {
			return state, err
		}
		deck.Status = status
		return touched(deck, meta), nil
	case *DeletedEvent:
		deck, ok := state.(GrantDeck)
		if !ok {
			return state, ErrInvalidState
		}
		return DeletedGrantDeck{ID: deck.ID, Version: meta.AggregateVersion}, nil
	default:
		return state, ErrInvalidState
	}
}

func touched(deck GrantDeck, meta EventMeta) GrantDeck {
	deck.Version = meta.AggregateVersion
	return deck
}
