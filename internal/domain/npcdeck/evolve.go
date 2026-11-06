package npcdeck

import (
	"maps"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

// 状態を書き換えないよう、カードの一覧は写してから変える
func Evolve(state State, event Event) (State, error) {
	meta := event.GetMeta()
	switch e := event.(type) {
	case *RegisteredEvent:
		if _, ok := state.(UnregisteredNPCDeck); !ok {
			return state, ErrInvalidState
		}
		cards := make(map[card.Ref]card.DeckQuantity, len(e.Cards))
		for _, entry := range e.Cards {
			cards[entry.Card] = entry.Quantity
		}
		return NPCDeck{
			ID:       meta.EntityID,
			Version:  meta.AggregateVersion,
			PublicID: e.PublicID,
			Name:     e.Name,
			Status:   StatusDraft,
			Cards:    cards,
		}, nil
	case *RenamedEvent:
		npcDeck, ok := state.(NPCDeck)
		if !ok {
			return state, ErrInvalidState
		}
		npcDeck.Name = e.Name
		return touched(npcDeck, meta), nil
	case *CardCountsChangedEvent:
		npcDeck, ok := state.(NPCDeck)
		if !ok {
			return state, ErrInvalidState
		}
		cards := maps.Clone(npcDeck.Cards)
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
		npcDeck.Cards = cards
		return touched(npcDeck, meta), nil
	case *PublishedEvent:
		npcDeck, ok := state.(NPCDeck)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := npcDeck.Status.Publish()
		if err != nil {
			return state, err
		}
		npcDeck.Status = status
		return touched(npcDeck, meta), nil
	case *RetiredEvent:
		npcDeck, ok := state.(NPCDeck)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := npcDeck.Status.Retire()
		if err != nil {
			return state, err
		}
		npcDeck.Status = status
		return touched(npcDeck, meta), nil
	case *DeletedEvent:
		npcDeck, ok := state.(NPCDeck)
		if !ok {
			return state, ErrInvalidState
		}
		return DeletedNPCDeck{ID: npcDeck.ID, Version: meta.AggregateVersion}, nil
	default:
		return state, ErrInvalidState
	}
}

func touched(npcDeck NPCDeck, meta EventMeta) NPCDeck {
	npcDeck.Version = meta.AggregateVersion
	return npcDeck
}
