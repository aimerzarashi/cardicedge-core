package startergrant

import (
	"bytes"
	"slices"
	"time"
)

// 中で time.Now を呼ばず発生時刻を受け取るのは、テストで時刻を固定するため
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *StartCommand:
		if _, ok := state.(NotStartedStarterGrant); !ok {
			return nil, ErrAlreadyStarted
		}
		if len(c.Decks) != RequiredDeckCount {
			return nil, ErrInvalidDeckCount
		}
		return []Event{&StartedEvent{
			EventMeta: EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			Player:    c.Player,
			Cards:     collectionOf(c.Decks),
			Decks:     c.Decks,
		}}, nil
	case *RecordCollectionCommand:
		progress, done, err := inProgress(state)
		if err != nil || done {
			return nil, err
		}
		if progress.CollectionRecorded {
			return nil, nil
		}
		return withCompletion(progress, now, &CollectionRecordedEvent{
			EventMeta: EventMeta{EntityID: progress.ID, AggregateVersion: progress.Version + 1, OccurredAt: now},
		}), nil
	case *RecordDeckCommand:
		progress, done, err := inProgress(state)
		if err != nil || done {
			return nil, err
		}
		if progress.DeckRecorded[c.Number.Int()-1] {
			return nil, nil
		}
		return withCompletion(progress, now, &DeckRecordedEvent{
			EventMeta: EventMeta{EntityID: progress.ID, AggregateVersion: progress.Version + 1, OccurredAt: now},
			Number:    c.Number,
		}), nil
	default:
		return nil, ErrInvalidState
	}
}

// 終わった配布への記録は、2 回目が来ても害が無いよう、エラーにせずイベントも作らない（done が true）
func inProgress(state State) (progress InProgressStarterGrant, done bool, err error) {
	switch s := state.(type) {
	case InProgressStarterGrant:
		return s, false, nil
	case CompletedStarterGrant:
		return InProgressStarterGrant{}, true, nil
	default:
		return InProgressStarterGrant{}, false, ErrInvalidState
	}
}

// 記録のイベントで 3 つそろうなら、続けて終わったイベントを出す
func withCompletion(progress InProgressStarterGrant, now time.Time, event Event) []Event {
	next, err := Evolve(progress, event)
	if err != nil {
		return []Event{event}
	}
	after, ok := next.(InProgressStarterGrant)
	if !ok || !after.allRecorded() {
		return []Event{event}
	}
	return []Event{event, &CompletedEvent{
		EventMeta: EventMeta{EntityID: progress.ID, AggregateVersion: after.Version + 1, OccurredAt: now},
	}}
}

// カードごとに、2 つのデッキのうち多いほうの枚数。1 枚の所持カードを複数のデッキで使えるため、合計にはしない。
// 同じ中身を同じ値として比べられるよう、カードの ID の順に並べる
func collectionOf(decks []Deck) []CardCount {
	most := map[[16]byte]CardCount{}
	for _, deck := range decks {
		for _, c := range deck.Cards {
			if current, ok := most[c.CardID]; !ok || c.Count > current.Count {
				most[c.CardID] = c
			}
		}
	}
	cards := make([]CardCount, 0, len(most))
	for _, c := range most {
		cards = append(cards, c)
	}
	slices.SortFunc(cards, func(a, b CardCount) int {
		return bytes.Compare(a.CardID[:], b.CardID[:])
	})
	return cards
}
