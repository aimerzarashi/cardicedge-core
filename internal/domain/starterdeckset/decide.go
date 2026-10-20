package starterdeckset

import (
	"slices"
	"strings"
	"time"
)

// 写す中身が今の写しと全部同じ（順番は問わない）なら、イベントを作らない（版を進めず、画面で見た版を古くしないため）。
// 元の ID の組だけで比べないのは、同じプリセットデッキを選び直して写し直す（反映する）ため
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *SetCommand:
		if len(c.Decks) != RequiredDeckCount {
			return nil, ErrDeckCountMismatch
		}
		decks := slices.Clone(c.Decks)
		slices.SortFunc(decks, compareDecks)
		for i := 1; i < len(decks); i++ {
			if decks[i].source.presetDeckID == decks[i-1].source.presetDeckID {
				return nil, ErrDuplicatedDeck
			}
		}
		switch s := state.(type) {
		case UnsetStarterDeckSet:
			return []Event{&DecksSetEvent{
				EventMeta: EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
				PublicID:  c.PublicID,
				Decks:     decks,
				SetBy:     c.SetBy,
			}}, nil
		case StarterDeckSet:
			if slices.EqualFunc(s.Decks, decks, Deck.equal) {
				return nil, nil
			}
			return []Event{&DecksSetEvent{
				EventMeta: EventMeta{EntityID: s.ID, AggregateVersion: s.Version + 1, OccurredAt: now},
				PublicID:  s.PublicID,
				Decks:     decks,
				SetBy:     c.SetBy,
			}}, nil
		default:
			return nil, ErrInvalidState
		}
	default:
		return nil, ErrInvalidState
	}
}

func compareDecks(a, b Deck) int {
	return strings.Compare(a.source.presetDeckID.String(), b.source.presetDeckID.String())
}
