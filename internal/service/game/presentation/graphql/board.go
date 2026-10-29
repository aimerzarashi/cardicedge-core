package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql/model"
)

func fromQueryBoard(board query.Board) (*model.Board, error) {
	tiles := make([]*model.BoardTile, 0, len(board.Tiles))
	for _, tile := range board.Tiles {
		kind, err := fromBoardTileKind(tile.Kind)
		if err != nil {
			return nil, err
		}
		attribute, err := fromBoardTileAttribute(tile.Attribute)
		if err != nil {
			return nil, err
		}
		tiles = append(tiles, &model.BoardTile{X: tile.X, Y: tile.Y, Kind: kind, Attribute: attribute})
	}
	connections := make([]*model.BoardConnection, 0, len(board.Connections))
	for _, connection := range board.Connections {
		connections = append(connections, &model.BoardConnection{
			A: &model.BoardPosition{X: connection.A.X, Y: connection.A.Y},
			B: &model.BoardPosition{X: connection.B.X, Y: connection.B.Y},
		})
	}
	return &model.Board{ID: board.ID, Name: board.Name, Width: board.Width, Height: board.Height, Tiles: tiles, Connections: connections}, nil
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

// enum の値と種類の対応を明示する
func fromBoardTileKind(kind query.BoardTileKind) (model.BoardTileKind, error) {
	switch kind {
	case query.BoardTileKindLand:
		return model.BoardTileKindLand, nil
	case query.BoardTileKindBase:
		return model.BoardTileKindBase, nil
	case query.BoardTileKindCheckpoint:
		return model.BoardTileKindCheckpoint, nil
	default:
		return "", fmt.Errorf("graphql: unknown board tile kind: %q", kind)
	}
}

// 空（土地でないマス）は null
func fromBoardTileAttribute(attribute query.BoardTileAttribute) (*model.BoardTileAttribute, error) {
	var converted model.BoardTileAttribute
	switch attribute {
	case "":
		return nil, nil
	case query.BoardTileAttributeFire:
		converted = model.BoardTileAttributeFire
	case query.BoardTileAttributeEarth:
		converted = model.BoardTileAttributeEarth
	case query.BoardTileAttributeWater:
		converted = model.BoardTileAttributeWater
	case query.BoardTileAttributeWind:
		converted = model.BoardTileAttributeWind
	case query.BoardTileAttributeNeutral:
		converted = model.BoardTileAttributeNeutral
	case query.BoardTileAttributeMulti:
		converted = model.BoardTileAttributeMulti
	default:
		return nil, fmt.Errorf("graphql: unknown board tile attribute: %q", attribute)
	}
	return &converted, nil
}
