package npcdeck

import (
	"cmp"
	"slices"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

// 変えても同じになる操作（同じ名前・同じ枚数・公開中にもう一度公開する等）は、イベントを作らない（版を進めず、画面の版を古くしないため）。
// 名前・枚数の変更と削除は準備中だけ。同じ状態への公開・使用不可も、準備中でなくてよい
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredNPCDeck); !ok {
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
		npcDeck, err := draft(state)
		if err != nil {
			return nil, err
		}
		if npcDeck.Name == c.Name {
			return nil, nil
		}
		return []Event{&RenamedEvent{EventMeta: nextMeta(npcDeck, now), Name: c.Name, RenamedBy: c.RenamedBy}}, nil
	case *SetCardCountsCommand:
		npcDeck, err := draft(state)
		if err != nil {
			return nil, err
		}
		for _, count := range c.Counts {
			if count.Int() > c.Rules.MaxCardDuplicates {
				return nil, ErrCardCountOutOfRange
			}
		}
		changes := cardCountChanges(npcDeck, c.Counts)
		if len(changes) == 0 {
			return nil, nil
		}
		total := npcDeck.TotalCount()
		for _, change := range changes {
			total += change.Delta
		}
		if total > c.Rules.TotalCardCount {
			return nil, ErrTotalCountTooLarge
		}
		return []Event{&CardCountsChangedEvent{EventMeta: nextMeta(npcDeck, now), Changes: changes, ChangedBy: c.ChangedBy}}, nil
	case *PublishCommand:
		npcDeck, ok := state.(NPCDeck)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := npcDeck.Status.Publish()
		if err != nil || !changed {
			return nil, err
		}
		if npcDeck.TotalCount() != c.Rules.TotalCardCount {
			return nil, ErrTotalMismatch
		}
		return []Event{&PublishedEvent{EventMeta: nextMeta(npcDeck, now), PublishedBy: c.PublishedBy}}, nil
	case *RetireCommand:
		npcDeck, ok := state.(NPCDeck)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := npcDeck.Status.Retire()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&RetiredEvent{EventMeta: nextMeta(npcDeck, now), RetiredBy: c.RetiredBy}}, nil
	case *DeleteCommand:
		npcDeck, err := draft(state)
		if err != nil {
			return nil, err
		}
		return []Event{&DeletedEvent{EventMeta: nextMeta(npcDeck, now), DeletedBy: c.DeletedBy}}, nil
	default:
		return nil, ErrInvalidState
	}
}

// 準備中の NPC のデッキだけを返す。登録していない・削除したものは ErrInvalidState、準備中でなければ ErrNotDraft
func draft(state State) (NPCDeck, error) {
	npcDeck, ok := state.(NPCDeck)
	if !ok {
		return NPCDeck{}, ErrInvalidState
	}
	if npcDeck.Status != StatusDraft {
		return NPCDeck{}, ErrNotDraft
	}
	return npcDeck, nil
}

func nextMeta(npcDeck NPCDeck, now time.Time) EventMeta {
	return EventMeta{EntityID: npcDeck.ID, AggregateVersion: npcDeck.Version + 1, OccurredAt: now}
}

// map の順は決まらないため、種類・内部 ID の順に並べる（同じ操作から同じイベントを作るため）。0 枚のカードは持たない
func cardEntries(cards map[presetdeck.CardRef]presetdeck.CardCount) []CardEntry {
	var entries []CardEntry
	for card, count := range cards {
		if count.Int() > 0 {
			entries = append(entries, CardEntry{Card: card, Count: count})
		}
	}
	slices.SortFunc(entries, func(a, b CardEntry) int { return compareCards(a.Card, b.Card) })
	return entries
}

func cardCountChanges(npcDeck NPCDeck, counts map[presetdeck.CardRef]presetdeck.CardCount) []CardCountChange {
	var changes []CardCountChange
	for card, count := range counts {
		if delta := count.Int() - npcDeck.Cards[card].Int(); delta != 0 {
			changes = append(changes, CardCountChange{Card: card, Delta: delta})
		}
	}
	slices.SortFunc(changes, func(a, b CardCountChange) int { return compareCards(a.Card, b.Card) })
	return changes
}

func compareCards(a, b presetdeck.CardRef) int {
	return cmp.Or(cmp.Compare(a.EntityType(), b.EntityType()), cmp.Compare(a.ID(), b.ID()))
}
