package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/presentation/graphql/model"
)

func fromQueryBoard(board query.Board) (*model.Board, error) {
	status, err := fromBoardStatus(board.Status)
	if err != nil {
		return nil, err
	}
	tiles := make([]model.BoardTile, 0, len(board.Tiles))
	for _, tile := range board.Tiles {
		converted, err := fromBoardTile(tile)
		if err != nil {
			return nil, err
		}
		tiles = append(tiles, converted)
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

func toBoardLayoutInput(layoutInput model.ChangeBoardLayoutInput) (command.BoardLayoutInput, error) {
	tiles := make([]command.BoardTileInput, 0, len(layoutInput.Tiles))
	invalid := input.Errors{}
	for i, tile := range layoutInput.Tiles {
		converted, ok, err := toBoardTileInput(tile)
		if err != nil {
			return command.BoardLayoutInput{}, err
		}
		if !ok {
			invalid = append(invalid, &input.InvalidFormat{Field: fmt.Sprintf("tiles.%d", i)})
			continue
		}
		tiles = append(tiles, converted)
	}
	if len(invalid) > 0 {
		return command.BoardLayoutInput{}, invalid
	}
	connections := make([]command.BoardConnectionInput, 0, len(layoutInput.Connections))
	for _, connection := range layoutInput.Connections {
		connections = append(connections, command.BoardConnectionInput{
			A: command.NewBoardPosition(connection.A.X, connection.A.Y),
			B: command.NewBoardPosition(connection.B.X, connection.B.Y),
		})
	}
	return command.BoardLayoutInput{Width: layoutInput.Width, Height: layoutInput.Height, Tiles: tiles, Connections: connections}, nil
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

// 種類の対応を明示する（read model の kind が増えて写し忘れたら、黙って落とさずエラーにする）
func fromBoardTile(tile readmodel.BoardTile) (model.BoardTile, error) {
	switch tile.Kind {
	case readmodel.BoardTileKindLand:
		attribute, err := fromBoardTileAttribute(tile.Attribute)
		if err != nil {
			return nil, err
		}
		return model.LandTile{X: tile.X, Y: tile.Y, Attribute: attribute}, nil
	case readmodel.BoardTileKindBase:
		return model.BaseTile{X: tile.X, Y: tile.Y}, nil
	case readmodel.BoardTileKindCheckpoint:
		return model.CheckpointTile{X: tile.X, Y: tile.Y}, nil
	default:
		return nil, fmt.Errorf("graphql: unknown board tile kind: %q", tile.Kind)
	}
}

// @oneOf の「ちょうど 1 つ」は、gqlparser・gqlgen が変数で送られた値を確かめない（0 個・2 個でもここまで届く。結合テストで確かめた）ため、
// ここで確かめ、満たさなければ ok が false（入力の誤り）。GraphQL の errors にしないのは、ほかの入力の誤りと同じく項目ごとに示すため
func toBoardTileInput(tile *model.BoardTileInput) (command.BoardTileInput, bool, error) {
	switch {
	case tile.Land != nil && tile.Base == nil && tile.Checkpoint == nil:
		attribute, err := toBoardTileAttribute(tile.Land.Attribute)
		if err != nil {
			return command.BoardTileInput{}, false, err
		}
		return command.BoardTileInput{X: tile.Land.X, Y: tile.Land.Y, Kind: string(readmodel.BoardTileKindLand), Attribute: string(attribute)}, true, nil
	case tile.Base != nil && tile.Land == nil && tile.Checkpoint == nil:
		return command.BoardTileInput{X: tile.Base.X, Y: tile.Base.Y, Kind: string(readmodel.BoardTileKindBase)}, true, nil
	case tile.Checkpoint != nil && tile.Land == nil && tile.Base == nil:
		return command.BoardTileInput{X: tile.Checkpoint.X, Y: tile.Checkpoint.Y, Kind: string(readmodel.BoardTileKindCheckpoint)}, true, nil
	default:
		return command.BoardTileInput{}, false, nil
	}
}

func toBoardTileAttribute(attribute model.BoardTileAttribute) (readmodel.BoardTileAttribute, error) {
	switch attribute {
	case model.BoardTileAttributeFire:
		return readmodel.BoardTileAttributeFire, nil
	case model.BoardTileAttributeEarth:
		return readmodel.BoardTileAttributeEarth, nil
	case model.BoardTileAttributeWater:
		return readmodel.BoardTileAttributeWater, nil
	case model.BoardTileAttributeWind:
		return readmodel.BoardTileAttributeWind, nil
	case model.BoardTileAttributeNeutral:
		return readmodel.BoardTileAttributeNeutral, nil
	case model.BoardTileAttributeMulti:
		return readmodel.BoardTileAttributeMulti, nil
	default:
		return "", fmt.Errorf("graphql: unknown board tile attribute: %s", attribute)
	}
}

func fromBoardTileAttribute(attribute readmodel.BoardTileAttribute) (model.BoardTileAttribute, error) {
	switch attribute {
	case readmodel.BoardTileAttributeFire:
		return model.BoardTileAttributeFire, nil
	case readmodel.BoardTileAttributeEarth:
		return model.BoardTileAttributeEarth, nil
	case readmodel.BoardTileAttributeWater:
		return model.BoardTileAttributeWater, nil
	case readmodel.BoardTileAttributeWind:
		return model.BoardTileAttributeWind, nil
	case readmodel.BoardTileAttributeNeutral:
		return model.BoardTileAttributeNeutral, nil
	case readmodel.BoardTileAttributeMulti:
		return model.BoardTileAttributeMulti, nil
	default:
		return "", fmt.Errorf("graphql: unknown board tile attribute: %q", attribute)
	}
}
