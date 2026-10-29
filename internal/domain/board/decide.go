package board

import (
	"fmt"
	"time"
)

// 変えても同じになる操作（同じ名前・同じレイアウト・同じ既定のルール・公開中にもう一度公開する等）は、イベントを作らない（版を進めず、画面の版を古くしないため）。
// 名前・レイアウト・既定のルールの変更と削除は準備中だけ。同じ状態への公開・使用不可も、準備中でなくてよい
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredBoard); !ok {
			return nil, ErrInvalidState
		}
		return []Event{&RegisteredEvent{
			EventMeta:    EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:     c.PublicID,
			Name:         c.Name,
			Width:        c.Rules.Clamp(DefaultSize),
			Height:       c.Rules.Clamp(DefaultSize),
			DefaultRules: InitialDefaultRules(),
			RegisteredBy: c.RegisteredBy,
		}}, nil
	case *RenameCommand:
		board, err := draft(state)
		if err != nil {
			return nil, err
		}
		if board.Name == c.Name {
			return nil, nil
		}
		return []Event{&RenamedEvent{EventMeta: nextMeta(board, now), Name: c.Name, RenamedBy: c.RenamedBy}}, nil
	case *ChangeLayoutCommand:
		board, err := draft(state)
		if err != nil {
			return nil, err
		}
		if size := c.Layout.Size(); !c.Rules.Contains(size.Width()) || !c.Rules.Contains(size.Height()) {
			return nil, fmt.Errorf("%w: %dx%d", ErrSizeOutOfRange, size.Width(), size.Height())
		}
		event := diffLayout(board.Layout, c.Layout)
		if event == nil {
			return nil, nil
		}
		event.EventMeta = nextMeta(board, now)
		event.ChangedBy = c.ChangedBy
		return []Event{event}, nil
	case *ChangeDefaultRulesCommand:
		board, err := draft(state)
		if err != nil {
			return nil, err
		}
		if board.DefaultRules == c.DefaultRules {
			return nil, nil
		}
		return []Event{&DefaultRulesChangedEvent{EventMeta: nextMeta(board, now), DefaultRules: c.DefaultRules, ChangedBy: c.ChangedBy}}, nil
	case *PublishCommand:
		board, ok := state.(Board)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := board.Status.Publish()
		if err != nil || !changed {
			return nil, err
		}
		if readiness := board.Layout.Readiness(); !readiness.Publishable() {
			return nil, &NotPublishableError{Readiness: readiness}
		}
		return []Event{&PublishedEvent{EventMeta: nextMeta(board, now), PublishedBy: c.PublishedBy}}, nil
	case *RetireCommand:
		board, ok := state.(Board)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := board.Status.Retire()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&RetiredEvent{EventMeta: nextMeta(board, now), RetiredBy: c.RetiredBy}}, nil
	case *DeleteCommand:
		board, err := draft(state)
		if err != nil {
			return nil, err
		}
		return []Event{&DeletedEvent{EventMeta: nextMeta(board, now), DeletedBy: c.DeletedBy}}, nil
	default:
		return nil, ErrInvalidState
	}
}

// 準備中のボードだけを返す。登録していない・削除したものは ErrInvalidState、準備中でなければ ErrNotDraft
func draft(state State) (Board, error) {
	board, ok := state.(Board)
	if !ok {
		return Board{}, ErrInvalidState
	}
	if board.Status != StatusDraft {
		return Board{}, ErrNotDraft
	}
	return board, nil
}

func nextMeta(board Board, now time.Time) EventMeta {
	return EventMeta{EntityID: board.ID, AggregateVersion: board.Version + 1, OccurredAt: now}
}

// 変わらなければ nil。マスの座標は map の順が決まらないため、Tiles・Connections の並び（y → x、a → b）で比べて、
// 同じレイアウトの変更から同じイベントを作る
func diffLayout(before, after Layout) *LayoutChangedEvent {
	event := &LayoutChangedEvent{}
	if before.size != after.size {
		size := after.size
		event.Size = &size
	}
	for _, tile := range after.Tiles() {
		previous, ok := before.tiles[tile.Position]
		switch {
		case !ok:
			event.PlacedTiles = append(event.PlacedTiles, tile)
		case previous != tile:
			event.ChangedTiles = append(event.ChangedTiles, tile)
		}
	}
	for _, tile := range before.Tiles() {
		if _, ok := after.tiles[tile.Position]; !ok {
			event.RemovedTiles = append(event.RemovedTiles, tile.Position)
		}
	}
	for _, connection := range after.Connections() {
		if _, ok := before.connections[connection]; !ok {
			event.Connected = append(event.Connected, connection)
		}
	}
	for _, connection := range before.Connections() {
		if _, ok := after.connections[connection]; !ok {
			event.Disconnected = append(event.Disconnected, connection)
		}
	}
	if event.Size == nil && len(event.PlacedTiles) == 0 && len(event.RemovedTiles) == 0 && len(event.ChangedTiles) == 0 &&
		len(event.Connected) == 0 && len(event.Disconnected) == 0 {
		return nil
	}
	return event
}
