package unlock

import (
	"slices"
	"time"
)

// 中で time.Now を呼ばず発生時刻を受け取るのは、テストで時刻を固定するため
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *CreateCommand:
		if _, ok := state.(UncreatedUnlock); !ok {
			return nil, ErrAlreadyCreated
		}
		return []Event{&CreatedEvent{
			EventMeta: EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			Player:    c.Player,
		}}, nil
	case *UnlockCommand:
		unlock, ok := state.(Unlock)
		if !ok {
			return nil, ErrInvalidState
		}
		// 種類ごとに、新しく解放するものだけを渡された順で持ち、新しいものがある種類だけイベントを出す（版は出したイベントの数だけ進む）
		var events []Event
		version := unlock.Version
		if boards := newOnes(unlock.Boards, c.Boards); len(boards) > 0 {
			version++
			events = append(events, &BoardsUnlockedEvent{EventMeta: EventMeta{EntityID: unlock.ID, AggregateVersion: version, OccurredAt: now}, Boards: boards})
		}
		if npcAvatars := newOnes(unlock.NPCAvatars, c.NPCAvatars); len(npcAvatars) > 0 {
			version++
			events = append(events, &NPCAvatarsUnlockedEvent{EventMeta: EventMeta{EntityID: unlock.ID, AggregateVersion: version, OccurredAt: now}, NPCAvatars: npcAvatars})
		}
		return events, nil
	default:
		return nil, ErrInvalidState
	}
}

// requested のうち、unlocked に無いものを、渡された順で返す。同じコマンドの中の重なりも 1 つにする
func newOnes[T comparable](unlocked, requested []T) []T {
	var added []T
	for _, id := range requested {
		if !slices.Contains(unlocked, id) && !slices.Contains(added, id) {
			added = append(added, id)
		}
	}
	return added
}
