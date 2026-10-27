package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/presentation/graphql/model"
)

func fromQueryBoard(board query.Board) (*model.Board, error) {
	status, err := fromBoardStatus(board.Status)
	if err != nil {
		return nil, err
	}
	tiles := make([]*model.BoardTile, 0, len(board.Tiles))
	for _, tile := range board.Tiles {
		kind, err := fromBoardTileKind(tile.Kind)
		if err != nil {
			return nil, err
		}
		tiles = append(tiles, &model.BoardTile{X: tile.X, Y: tile.Y, Kind: kind})
	}
	connections := make([]*model.BoardConnection, 0, len(board.Connections))
	for _, connection := range board.Connections {
		connections = append(connections, &model.BoardConnection{
			A: &model.BoardPosition{X: connection.A.X, Y: connection.A.Y},
			B: &model.BoardPosition{X: connection.B.X, Y: connection.B.Y},
		})
	}
	return &model.Board{
		ID: board.ID, Name: board.Name, Status: status, Width: board.Width, Height: board.Height,
		Tiles: tiles, Connections: connections, Revision: int(board.Revision),
		RegisteredAt: board.RegisteredAt, UpdatedAt: board.UpdatedAt,
	}, nil
}

func toBoardsConnection(connection pagination.Connection[query.Board]) (*model.BoardsConnection, error) {
	edges := make([]*model.BoardsEdge, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		node, err := fromQueryBoard(edge.Node)
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.BoardsEdge{Cursor: edge.Cursor, Node: node})
	}
	pageInfo := &model.PageInfo{
		HasNextPage:     connection.PageInfo.HasNextPage,
		HasPreviousPage: connection.PageInfo.HasPreviousPage,
		StartCursor:     connection.PageInfo.StartCursor,
		EndCursor:       connection.PageInfo.EndCursor,
	}
	return &model.BoardsConnection{Edges: edges, PageInfo: pageInfo}, nil
}

// statuses の null（絞り込まない）と空のリスト（入力の誤り）は、application 層が見分けるため区別して渡す
func toBoardFilter(filter *model.BoardFilter) (query.BoardFilter, error) {
	if filter == nil {
		return query.BoardFilter{}, nil
	}
	var statuses []readmodel.BoardStatus
	if filter.Statuses != nil {
		statuses = make([]readmodel.BoardStatus, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			converted, err := toBoardStatus(status)
			if err != nil {
				return query.BoardFilter{}, err
			}
			statuses = append(statuses, converted)
		}
	}
	return query.BoardFilter{NameContains: filter.NameContains, Statuses: statuses}, nil
}

func toBoardLayoutInput(input model.ChangeBoardLayoutInput) (command.BoardLayoutInput, error) {
	tiles := make([]command.BoardTileInput, 0, len(input.Tiles))
	for _, tile := range input.Tiles {
		kind, err := toBoardTileKind(tile.Kind)
		if err != nil {
			return command.BoardLayoutInput{}, err
		}
		tiles = append(tiles, command.BoardTileInput{X: tile.X, Y: tile.Y, Kind: string(kind)})
	}
	connections := make([]command.BoardConnectionInput, 0, len(input.Connections))
	for _, connection := range input.Connections {
		connections = append(connections, command.BoardConnectionInput{
			A: command.NewBoardPosition(connection.A.X, connection.A.Y),
			B: command.NewBoardPosition(connection.B.X, connection.B.Y),
		})
	}
	return command.BoardLayoutInput{Width: input.Width, Height: input.Height, Tiles: tiles, Connections: connections}, nil
}

func fromBoardNotPublishable(err *command.BoardNotPublishableError) *model.BoardNotPublishableError {
	unreachable := make([]*model.BoardPosition, 0, len(err.UnreachableTiles))
	for _, pos := range err.UnreachableTiles {
		unreachable = append(unreachable, &model.BoardPosition{X: pos.X, Y: pos.Y})
	}
	return &model.BoardNotPublishableError{
		Message: "board is not publishable", BaseCount: err.BaseCount, CheckpointCount: err.CheckpointCount, UnreachableTiles: unreachable,
	}
}

// enum の値と状態の対応を明示する（toCardStatus と同じ）
func toBoardStatus(status model.BoardStatus) (readmodel.BoardStatus, error) {
	switch status {
	case model.BoardStatusDraft:
		return readmodel.BoardStatusDraft, nil
	case model.BoardStatusPublished:
		return readmodel.BoardStatusPublished, nil
	case model.BoardStatusRetired:
		return readmodel.BoardStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown board status: %s", status)
	}
}

func fromBoardStatus(status readmodel.BoardStatus) (model.BoardStatus, error) {
	switch status {
	case readmodel.BoardStatusDraft:
		return model.BoardStatusDraft, nil
	case readmodel.BoardStatusPublished:
		return model.BoardStatusPublished, nil
	case readmodel.BoardStatusRetired:
		return model.BoardStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown board status: %q", status)
	}
}

func toBoardTileKind(kind model.BoardTileKind) (readmodel.BoardTileKind, error) {
	switch kind {
	case model.BoardTileKindLand:
		return readmodel.BoardTileKindLand, nil
	case model.BoardTileKindBase:
		return readmodel.BoardTileKindBase, nil
	case model.BoardTileKindCheckpoint:
		return readmodel.BoardTileKindCheckpoint, nil
	default:
		return "", fmt.Errorf("graphql: unknown board tile kind: %s", kind)
	}
}

func fromBoardTileKind(kind readmodel.BoardTileKind) (model.BoardTileKind, error) {
	switch kind {
	case readmodel.BoardTileKindLand:
		return model.BoardTileKindLand, nil
	case readmodel.BoardTileKindBase:
		return model.BoardTileKindBase, nil
	case readmodel.BoardTileKindCheckpoint:
		return model.BoardTileKindCheckpoint, nil
	default:
		return "", fmt.Errorf("graphql: unknown board tile kind: %q", kind)
	}
}
