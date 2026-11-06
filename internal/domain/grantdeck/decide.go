package grantdeck

import (
	"slices"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

// 変えても同じになる操作（同じ名前・同じ枚数・公開中にもう一度公開する等）は、イベントを作らない（版を進めず、画面の版を古くしないため）。
// 名前・枚数の変更と削除は準備中だけ。同じ状態への公開・使用不可も、準備中でなくてよい
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredGrantDeck); !ok {
			return nil, ErrInvalidState
		}
		return []Event{&RegisteredEvent{
			EventMeta:    EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:     c.PublicID,
			Name:         c.Name,
			Source:       c.Source,
			Cards:        cardEntries(c.Cards),
			RegisteredBy: c.RegisteredBy,
		}}, nil
	case *RenameCommand:
		deck, err := draft(state)
		if err != nil {
			return nil, err
		}
		if deck.Name == c.Name {
			return nil, nil
		}
		return []Event{&RenamedEvent{EventMeta: nextMeta(deck, now), Name: c.Name, RenamedBy: c.RenamedBy}}, nil
	case *SetCardCountsCommand:
		deck, err := draft(state)
		if err != nil {
			return nil, err
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
		if total > c.Rules.TotalCardCount.Int() {
			return nil, ErrTotalCountTooLarge
		}
		return []Event{&CardCountsChangedEvent{EventMeta: nextMeta(deck, now), Changes: changes, ChangedBy: c.ChangedBy}}, nil
	case *PublishCommand:
		deck, ok := state.(GrantDeck)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := deck.Status.Publish()
		if err != nil || !changed {
			return nil, err
		}
		if deck.TotalCount() != c.Rules.TotalCardCount.Int() {
			return nil, ErrTotalMismatch
		}
		return []Event{&PublishedEvent{EventMeta: nextMeta(deck, now), PublishedBy: c.PublishedBy}}, nil
	case *RetireCommand:
		deck, ok := state.(GrantDeck)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := deck.Status.Retire()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&RetiredEvent{EventMeta: nextMeta(deck, now), RetiredBy: c.RetiredBy}}, nil
	case *DeleteCommand:
		deck, err := draft(state)
		if err != nil {
			return nil, err
		}
		return []Event{&DeletedEvent{EventMeta: nextMeta(deck, now), DeletedBy: c.DeletedBy}}, nil
	default:
		return nil, ErrInvalidState
	}
}

// 準備中の配布デッキだけを返す。登録していない・削除したものは ErrInvalidState、準備中でなければ ErrNotDraft
func draft(state State) (GrantDeck, error) {
	deck, ok := state.(GrantDeck)
	if !ok {
		return GrantDeck{}, ErrInvalidState
	}
	if deck.Status != StatusDraft {
		return GrantDeck{}, ErrNotDraft
	}
	return deck, nil
}

func nextMeta(deck GrantDeck, now time.Time) EventMeta {
	return EventMeta{EntityID: deck.ID, AggregateVersion: deck.Version + 1, OccurredAt: now}
}

// map の順は決まらないため、種類・内部 ID の順に並べる（同じ操作から同じイベントを作るため）
func cardEntries(cards map[card.Ref]card.DeckQuantity) []DeckCard {
	var entries []DeckCard
	for ref, quantity := range cards {
		entries = append(entries, DeckCard{Card: ref, Quantity: quantity})
	}
	slices.SortFunc(entries, func(a, b DeckCard) int { return a.Card.Compare(b.Card) })
	return entries
}

func cardCountChanges(deck GrantDeck, set map[card.Ref]card.DeckQuantity, remove []card.Ref) ([]CardCountChange, error) {
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
	return slices.CompactFunc(changes, func(a, b CardCountChange) bool { return a.Card == b.Card }), nil
}
