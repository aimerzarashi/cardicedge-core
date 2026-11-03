package startergrant

import (
	"bytes"
	"slices"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

// 中で time.Now を呼ばず発生時刻を受け取るのは、テストで時刻を固定するため
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *StartCommand:
		if _, ok := state.(NotStartedStarterGrant); !ok {
			return nil, ErrAlreadyStarted
		}
		if len(c.Decks) < 1 {
			return nil, ErrInvalidDeckCount
		}
		return []Event{&StartedEvent{
			EventMeta:    EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			Player:       c.Player,
			Cards:        collectionOf(c.Decks),
			Decks:        c.Decks,
			UnlockBoards: c.UnlockBoards, UnlockNPCAvatars: c.UnlockNPCAvatars,
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
	case *RecordUnlockCommand:
		progress, done, err := inProgress(state)
		if err != nil || done {
			return nil, err
		}
		if progress.UnlockRecorded {
			return nil, nil
		}
		return withCompletion(progress, now, &UnlockRecordedEvent{
			EventMeta: EventMeta{EntityID: progress.ID, AggregateVersion: progress.Version + 1, OccurredAt: now},
		}), nil
	case *RecordDeckCommand:
		progress, done, err := inProgress(state)
		if err != nil || done {
			return nil, err
		}
		if c.Number.Int() > len(progress.DeckRecorded) {
			return nil, ErrInvalidDeckNumber
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

// 記録のイベントで所持カード・すべてのデッキ・解放がそろうなら、続けて終わったイベントを出す
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

// カードごとに、配るデッキのうち多いほうの枚数。1 枚の所持カードを複数のデッキで使えるため、合計にはしない。
// 同じ中身を同じ値として比べられるよう、カードの ID の順に並べる
func collectionOf(decks []Deck) []card.Count {
	most := map[[16]byte]card.Count{}
	for _, deck := range decks {
		for _, c := range deck.Cards {
			if current, ok := most[c.CardID]; !ok || c.Count > current.Count {
				most[c.CardID] = c
			}
		}
	}
	cards := make([]card.Count, 0, len(most))
	for _, c := range most {
		cards = append(cards, c)
	}
	slices.SortFunc(cards, func(a, b card.Count) int {
		return bytes.Compare(a.CardID[:], b.CardID[:])
	})
	return cards
}
