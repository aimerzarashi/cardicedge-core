package starterdeck

import (
	"maps"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

// 状態を書き換えないよう、カードの一覧は写してから変える
func Evolve(state State, event Event) (State, error) {
	meta := event.GetMeta()
	switch e := event.(type) {
	case *RegisteredEvent:
		if _, ok := state.(UnregisteredStarterDeck); !ok {
			return state, ErrInvalidState
		}
		cards := make(map[presetdeck.CardRef]presetdeck.CardCount, len(e.Cards))
		for _, entry := range e.Cards {
			cards[entry.Card] = entry.Count
		}
		return StarterDeck{
			ID:           meta.EntityID,
			Version:      meta.AggregateVersion,
			PublicID:     e.PublicID,
			Name:         e.Name,
			Status:       StatusDraft,
			Source:       e.Source,
			Cards:        cards,
			RegisteredBy: e.RegisteredBy,
			RegisteredAt: meta.OccurredAt,
			UpdatedAt:    meta.OccurredAt,
		}, nil
	case *RenamedEvent:
		deck, ok := state.(StarterDeck)
		if !ok {
			return state, ErrInvalidState
		}
		deck.Name = e.Name
		return touched(deck, meta), nil
	case *CardCountsChangedEvent:
		deck, ok := state.(StarterDeck)
		if !ok {
			return state, ErrInvalidState
		}
		cards := maps.Clone(deck.Cards)
		for _, change := range e.Changes {
			count, err := presetdeck.NewCardCount(cards[change.Card].Int() + change.Delta)
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
	case *PublishedEvent:
		deck, ok := state.(StarterDeck)
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
		deck, ok := state.(StarterDeck)
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
		deck, ok := state.(StarterDeck)
		if !ok {
			return state, ErrInvalidState
		}
		return DeletedStarterDeck{ID: deck.ID, Version: meta.AggregateVersion}, nil
	default:
		return state, ErrInvalidState
	}
}

func touched(deck StarterDeck, meta EventMeta) StarterDeck {
	deck.Version = meta.AggregateVersion
	deck.UpdatedAt = meta.OccurredAt
	return deck
}
