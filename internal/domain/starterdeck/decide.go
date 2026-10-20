package starterdeck

import (
	"cmp"
	"slices"
	"time"
)

// 変えても同じになる操作（同じ名前・同じ枚数）は、イベントを作らない（版を進めず、画面の版を古くしないため）
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredStarterDeck); !ok {
			return nil, ErrInvalidState
		}
		return []Event{&RegisteredEvent{
			EventMeta:    EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:     c.PublicID,
			Name:         c.Name,
			RegisteredBy: c.RegisteredBy,
		}}, nil
	case *RenameCommand:
		deck, ok := state.(StarterDeck)
		if !ok {
			return nil, ErrInvalidState
		}
		if deck.Name == c.Name {
			return nil, nil
		}
		return []Event{&RenamedEvent{EventMeta: nextMeta(deck, now), Name: c.Name, RenamedBy: c.RenamedBy}}, nil
	case *SetCardCountsCommand:
		deck, ok := state.(StarterDeck)
		if !ok {
			return nil, ErrInvalidState
		}
		changes := cardCountChanges(deck, c.Counts)
		if len(changes) == 0 {
			return nil, nil
		}
		total := deck.TotalCount()
		for _, change := range changes {
			total += change.Delta
		}
		if total > MaxTotalCount {
			return nil, ErrTotalCountTooLarge
		}
		return []Event{&CardCountsChangedEvent{EventMeta: nextMeta(deck, now), Changes: changes, ChangedBy: c.ChangedBy}}, nil
	case *DeleteCommand:
		deck, ok := state.(StarterDeck)
		if !ok {
			return nil, ErrInvalidState
		}
		return []Event{&DeletedEvent{EventMeta: nextMeta(deck, now), DeletedBy: c.DeletedBy}}, nil
	default:
		return nil, ErrInvalidState
	}
}

func nextMeta(deck StarterDeck, now time.Time) EventMeta {
	return EventMeta{EntityID: deck.ID, AggregateVersion: deck.Version + 1, OccurredAt: now}
}

// map の順は決まらないため、種類・内部 ID の順に並べる（同じ操作から同じイベントを作るため）
func cardCountChanges(deck StarterDeck, counts map[CardRef]CardCount) []CardCountChange {
	var changes []CardCountChange
	for card, count := range counts {
		if delta := count.Int() - deck.Cards[card].Int(); delta != 0 {
			changes = append(changes, CardCountChange{Card: card, Delta: delta})
		}
	}
	slices.SortFunc(changes, func(a, b CardCountChange) int {
		return cmp.Or(cmp.Compare(a.Card.EntityType(), b.Card.EntityType()), cmp.Compare(a.Card.ID(), b.Card.ID()))
	})
	return changes
}
