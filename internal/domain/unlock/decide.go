package unlock

import (
	"slices"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
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
	case *UnlockBoardsCommand:
		unlock, ok := state.(Unlock)
		if !ok {
			return nil, ErrInvalidState
		}
		// 新しく解放するものだけを、渡された順で持つ。同じコマンドの中の重なりも 1 つにする。新しいものが無ければイベントを作らない
		var unlocked []board.ID
		for _, id := range c.Boards {
			if !slices.Contains(unlock.Boards, id) && !slices.Contains(unlocked, id) {
				unlocked = append(unlocked, id)
			}
		}
		if len(unlocked) == 0 {
			return nil, nil
		}
		return []Event{&BoardsUnlockedEvent{
			EventMeta: EventMeta{EntityID: unlock.ID, AggregateVersion: unlock.Version + 1, OccurredAt: now},
			Boards:    unlocked,
		}}, nil
	default:
		return nil, ErrInvalidState
	}
}
