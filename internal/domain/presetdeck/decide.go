package presetdeck

import (
	"slices"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

// 変えても同じになる操作（同じ名前・同じ枚数）は、イベントを作らない（版を進めず、画面の版を古くしないため）
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredPresetDeck); !ok {
			return nil, ErrInvalidState
		}
		return []Event{&RegisteredEvent{
			EventMeta:    EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:     c.PublicID,
			Name:         c.Name,
			RegisteredBy: c.RegisteredBy,
		}}, nil
	case *RenameCommand:
		deck, ok := state.(PresetDeck)
		if !ok {
			return nil, ErrInvalidState
		}
		if deck.Name == c.Name {
			return nil, nil
		}
		return []Event{&RenamedEvent{EventMeta: nextMeta(deck, now), Name: c.Name, RenamedBy: c.RenamedBy}}, nil
	case *SetCardCountsCommand:
		deck, ok := state.(PresetDeck)
		if !ok {
			return nil, ErrInvalidState
		}
		changes, err := cardCountChanges(deck, c.Set, c.Remove)
		if err != nil {
			return nil, err
		}
		if len(changes) == 0 {
			return nil, nil
		}
		total := deck.TotalCount()
		for _, change := range changes {
			total += change.Delta
		}
		if total > c.Rules.TotalCardCount {
			return nil, ErrTotalCountTooLarge
		}
		return []Event{&CardCountsChangedEvent{EventMeta: nextMeta(deck, now), Changes: changes, ChangedBy: c.ChangedBy}}, nil
	case *DeleteCommand:
		deck, ok := state.(PresetDeck)
		if !ok {
			return nil, ErrInvalidState
		}
		return []Event{&DeletedEvent{EventMeta: nextMeta(deck, now), DeletedBy: c.DeletedBy}}, nil
	default:
		return nil, ErrInvalidState
	}
}

func nextMeta(deck PresetDeck, now time.Time) EventMeta {
	return EventMeta{EntityID: deck.ID, AggregateVersion: deck.Version + 1, OccurredAt: now}
}

// map の順は決まらないため、種類・内部 ID の順に並べる（同じ操作から同じイベントを作るため）
func cardCountChanges(deck PresetDeck, set map[card.Ref]card.DeckQuantity, remove []card.Ref) ([]CardCountChange, error) {
	var changes []CardCountChange
	for ref, quantity := range set {
		if delta := quantity.Int() - deck.Cards[ref].Int(); delta != 0 {
			changes = append(changes, CardCountChange{Card: ref, Delta: delta})
		}
	}
	for _, ref := range remove {
		if _, ok := set[ref]; ok {
			return nil, ErrCardConflict
		}
		if current := deck.Cards[ref].Int(); current != 0 {
			changes = append(changes, CardCountChange{Card: ref, Delta: -current})
		}
	}
	slices.SortFunc(changes, func(a, b CardCountChange) int { return a.Card.Compare(b.Card) })
	// 同じカードを 2 回外す指定は 1 回にする（差分を重ねて、持っている枚数より多く引かないため）
	changes = slices.CompactFunc(changes, func(a, b CardCountChange) bool { return a.Card == b.Card })
	return changes, nil
}
