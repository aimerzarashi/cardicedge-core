package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql/model"
)

func fromQueryBoard(board query.Board) (*model.Board, error) {
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
	pairs := make([]*model.BoardTilePair, 0, len(board.Pairs))
	for _, pair := range board.Pairs {
		pairs = append(pairs, &model.BoardTilePair{
			A: &model.BoardPosition{X: pair.A.X, Y: pair.A.Y},
			B: &model.BoardPosition{X: pair.B.X, Y: pair.B.Y},
		})
	}
	defaultRules := &model.BoardDefaultRules{
		TargetMagic: board.DefaultRules.TargetMagic, StartingMagic: board.DefaultRules.StartingMagic,
		LapBonus: board.DefaultRules.LapBonus, CheckpointBonus: board.DefaultRules.CheckpointBonus,
		RoundLimit: board.DefaultRules.RoundLimit, DiceMin: board.DefaultRules.DiceMin, DiceMax: board.DefaultRules.DiceMax,
	}
	c := board.TileCounts
	tileCounts := &model.BoardTileCounts{
		Land: c.Land, Base: c.Base, Checkpoint: c.Checkpoint,
		Fire: c.Fire, Earth: c.Earth, Water: c.Water, Wind: c.Wind, Neutral: c.Neutral, Multi: c.Multi,
		Warp: c.Warp, Bridge: c.Bridge, Sanctuary: c.Sanctuary, Shrine: c.Shrine, ProphecyShop: c.ProphecyShop, CardShop: c.CardShop,
	}
	return &model.Board{ID: board.ID, Name: board.Name, Width: board.Width, Height: board.Height, DefaultRules: defaultRules, TileCounts: tileCounts, Tiles: tiles, Connections: connections, Pairs: pairs}, nil
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

// 種類の対応を明示する（read model の kind が増えて写し忘れたら、黙って落とさずエラーにする）
func fromBoardTile(tile query.BoardTile) (model.BoardTile, error) {
	switch tile.Kind {
	case query.BoardTileKindLand:
		attribute, err := fromBoardTileAttribute(tile.Attribute)
		if err != nil {
			return nil, err
		}
		return model.LandTile{X: tile.X, Y: tile.Y, Attribute: attribute}, nil
	case query.BoardTileKindBase:
		return model.BaseTile{X: tile.X, Y: tile.Y}, nil
	case query.BoardTileKindCheckpoint:
		return model.CheckpointTile{X: tile.X, Y: tile.Y}, nil
	case query.BoardTileKindWarp:
		return model.WarpTile{X: tile.X, Y: tile.Y}, nil
	case query.BoardTileKindBridge:
		return model.BridgeTile{X: tile.X, Y: tile.Y}, nil
	case query.BoardTileKindSanctuary:
		return model.SanctuaryTile{X: tile.X, Y: tile.Y}, nil
	case query.BoardTileKindShrine:
		return model.ShrineTile{X: tile.X, Y: tile.Y}, nil
	case query.BoardTileKindProphecyShop:
		return model.ProphecyShopTile{X: tile.X, Y: tile.Y}, nil
	case query.BoardTileKindCardShop:
		return model.CardShopTile{X: tile.X, Y: tile.Y}, nil
	default:
		return nil, fmt.Errorf("graphql: unknown board tile kind: %q", tile.Kind)
	}
}

// 土地は必ず属性を持つので、空は不明な値と同じくエラーにする
func fromBoardTileAttribute(attribute query.BoardTileAttribute) (model.BoardTileAttribute, error) {
	switch attribute {
	case query.BoardTileAttributeFire:
		return model.BoardTileAttributeFire, nil
	case query.BoardTileAttributeEarth:
		return model.BoardTileAttributeEarth, nil
	case query.BoardTileAttributeWater:
		return model.BoardTileAttributeWater, nil
	case query.BoardTileAttributeWind:
		return model.BoardTileAttributeWind, nil
	case query.BoardTileAttributeNeutral:
		return model.BoardTileAttributeNeutral, nil
	case query.BoardTileAttributeMulti:
		return model.BoardTileAttributeMulti, nil
	default:
		return "", fmt.Errorf("graphql: unknown board tile attribute: %q", attribute)
	}
}
