package board

import "maps"

// 状態を書き換えないよう、マスとつながりは写してから変える
func Evolve(state State, event Event) (State, error) {
	meta := event.GetMeta()
	switch e := event.(type) {
	case *RegisteredEvent:
		if _, ok := state.(UnregisteredBoard); !ok {
			return state, ErrInvalidState
		}
		// NewSize を通さない（範囲を後で下げても、保存済みのボードを読み直せるように。Size の UnmarshalJSON と同じ）
		size := Size{width: e.Width, height: e.Height}
		return Board{
			ID:           meta.EntityID,
			Version:      meta.AggregateVersion,
			PublicID:     e.PublicID,
			Name:         e.Name,
			Status:       StatusDraft,
			Layout:       EmptyLayout(size),
			DefaultRules: e.DefaultRules,
			RegisteredBy: e.RegisteredBy,
			RegisteredAt: meta.OccurredAt,
			UpdatedAt:    meta.OccurredAt,
		}, nil
	case *RenamedEvent:
		board, ok := state.(Board)
		if !ok {
			return state, ErrInvalidState
		}
		board.Name = e.Name
		return touched(board, meta), nil
	case *LayoutChangedEvent:
		board, ok := state.(Board)
		if !ok {
			return state, ErrInvalidState
		}
		layout, err := applyLayoutChange(board.Layout, e)
		if err != nil {
			return state, err
		}
		board.Layout = layout
		return touched(board, meta), nil
	case *DefaultRulesChangedEvent:
		board, ok := state.(Board)
		if !ok {
			return state, ErrInvalidState
		}
		board.DefaultRules = e.DefaultRules
		return touched(board, meta), nil
	case *PublishedEvent:
		board, ok := state.(Board)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := board.Status.Publish()
		if err != nil {
			return state, err
		}
		board.Status = status
		return touched(board, meta), nil
	case *RetiredEvent:
		board, ok := state.(Board)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := board.Status.Retire()
		if err != nil {
			return state, err
		}
		board.Status = status
		return touched(board, meta), nil
	case *DeletedEvent:
		board, ok := state.(Board)
		if !ok {
			return state, ErrInvalidState
		}
		return DeletedBoard{ID: board.ID, Version: meta.AggregateVersion}, nil
	default:
		return state, ErrInvalidState
	}
}

// 差分を当てた結果も NewLayout の決まりで確かめる。保存済みのイベントが決まりを破る状態を作るなら、黙って受け入れず止める
func applyLayoutChange(before Layout, e *LayoutChangedEvent) (Layout, error) {
	size := before.size
	if e.Size != nil {
		size = *e.Size
	}
	tiles := maps.Clone(before.tiles)
	connections := maps.Clone(before.connections)
	for _, pos := range e.RemovedTiles {
		delete(tiles, pos)
	}
	for _, tile := range e.PlacedTiles {
		tiles[tile.Position] = tile
	}
	for _, tile := range e.ChangedTiles {
		tiles[tile.Position] = tile
	}
	for _, connection := range e.Disconnected {
		delete(connections, connection)
	}
	for _, connection := range e.Connected {
		connections[connection] = struct{}{}
	}
	return NewLayout(size, tilesOf(tiles), connectionsOf(connections))
}

func tilesOf(tiles map[Position]Tile) []Tile {
	result := make([]Tile, 0, len(tiles))
	for _, tile := range tiles {
		result = append(result, tile)
	}
	return result
}

func connectionsOf(connections map[Connection]struct{}) []Connection {
	result := make([]Connection, 0, len(connections))
	for connection := range connections {
		result = append(result, connection)
	}
	return result
}

func touched(board Board, meta EventMeta) Board {
	board.Version = meta.AggregateVersion
	board.UpdatedAt = meta.OccurredAt
	return board
}
