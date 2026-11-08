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
	problems := make([]model.BoardPublishProblem, 0, len(board.PublishProblems))
	for _, problem := range board.PublishProblems {
		converted, err := fromBoardPublishProblem(string(problem.Kind), fromReadmodelPositions(problem.Positions))
		if err != nil {
			return nil, err
		}
		problems = append(problems, converted)
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
	return &model.Board{
		ID: board.ID, Name: board.Name, Status: status, Width: board.Width, Height: board.Height,
		Tiles: tiles, Connections: connections, Pairs: pairs, DefaultRules: fromBoardDefaultRules(board.DefaultRules), TileCounts: fromBoardTileCounts(board.TileCounts), PublishProblems: problems, Revision: int(board.Revision),
		RegisteredAt: board.RegisteredAt, UpdatedAt: board.UpdatedAt,
	}, nil
}

func fromBoardDefaultRules(rules readmodel.BoardDefaultRules) *model.BoardDefaultRules {
	return &model.BoardDefaultRules{
		TargetMagic: rules.TargetMagic, StartingMagic: rules.StartingMagic, LapBonus: rules.LapBonus, CheckpointBonus: rules.CheckpointBonus,
		RoundLimit: rules.RoundLimit, DiceMin: rules.DiceMin, DiceMax: rules.DiceMax,
	}
}

func fromBoardTileCounts(counts readmodel.BoardTileCounts) *model.BoardTileCounts {
	return &model.BoardTileCounts{
		Land: counts.Land, Base: counts.Base, Checkpoint: counts.Checkpoint,
		Fire: counts.Fire, Earth: counts.Earth, Water: counts.Water, Wind: counts.Wind, Neutral: counts.Neutral, Multi: counts.Multi,
		Warp: counts.Warp, Bridge: counts.Bridge, Sanctuary: counts.Sanctuary, Shrine: counts.Shrine, ProphecyShop: counts.ProphecyShop, CardShop: counts.CardShop,
	}
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

// 保存（changeBoardLayout）と判定（checkBoardLayout）の入力は、レイアウトの 5 項目が同じ。写し方を共有する
func toBoardLayoutInput(layoutInput model.ChangeBoardLayoutInput) (command.BoardLayoutInput, error) {
	return toBoardLayout(layoutInput.Width, layoutInput.Height, layoutInput.Tiles, layoutInput.Connections, layoutInput.Pairs)
}

func toCheckBoardLayoutInput(layoutInput model.CheckBoardLayoutInput) (command.BoardLayoutInput, error) {
	return toBoardLayout(layoutInput.Width, layoutInput.Height, layoutInput.Tiles, layoutInput.Connections, layoutInput.Pairs)
}

func toBoardLayout(width, height int, inputTiles []*model.BoardTileInput, inputConnections []*model.BoardConnectionInput, inputPairs []*model.BoardTilePairInput) (command.BoardLayoutInput, error) {
	tiles := make([]command.BoardTileInput, 0, len(inputTiles))
	invalid := input.Errors{}
	for i, tile := range inputTiles {
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
	connections := make([]command.BoardConnectionInput, 0, len(inputConnections))
	for _, connection := range inputConnections {
		connections = append(connections, command.BoardConnectionInput{
			A: command.NewBoardPosition(connection.A.X, connection.A.Y),
			B: command.NewBoardPosition(connection.B.X, connection.B.Y),
		})
	}
	pairs := make([]command.BoardPairInput, 0, len(inputPairs))
	for _, pair := range inputPairs {
		pairs = append(pairs, command.BoardPairInput{
			A: command.NewBoardPosition(pair.A.X, pair.A.Y),
			B: command.NewBoardPosition(pair.B.X, pair.B.Y),
		})
	}
	return command.BoardLayoutInput{Width: width, Height: height, Tiles: tiles, Connections: connections, Pairs: pairs}, nil
}

func fromBoardNotPublishable(notPublishable *command.BoardNotPublishableError) (*model.BoardNotPublishableError, error) {
	problems, err := fromCommandPublishProblems(notPublishable.Problems)
	if err != nil {
		return nil, err
	}
	return &model.BoardNotPublishableError{Message: "board is not publishable", Problems: problems}, nil
}

func fromCommandPublishProblems(commandProblems []command.BoardPublishProblem) ([]model.BoardPublishProblem, error) {
	problems := make([]model.BoardPublishProblem, 0, len(commandProblems))
	for _, problem := range commandProblems {
		converted, err := fromBoardPublishProblem(string(problem.Kind), fromCommandPositions(problem.Positions))
		if err != nil {
			return nil, err
		}
		problems = append(problems, converted)
	}
	return problems, nil
}

func fromCommandPositions(positions []command.BoardPosition) []*model.BoardPosition {
	result := make([]*model.BoardPosition, 0, len(positions))
	for _, pos := range positions {
		result = append(result, &model.BoardPosition{X: pos.X, Y: pos.Y})
	}
	return result
}

func fromReadmodelPositions(positions []readmodel.BoardPosition) []*model.BoardPosition {
	result := make([]*model.BoardPosition, 0, len(positions))
	for _, pos := range positions {
		result = append(result, &model.BoardPosition{X: pos.X, Y: pos.Y})
	}
	return result
}

// 種類は read model（読む側）と command（公開に失敗した側）で同じ文字列。理由を足したのに写し方を足し忘れたら、黙って捨てずに止める
func fromBoardPublishProblem(kind string, positions []*model.BoardPosition) (model.BoardPublishProblem, error) {
	switch kind {
	case string(readmodel.BoardPublishProblemBaseCount):
		return &model.BaseCountProblem{Message: "board must have exactly one base", Positions: positions}, nil
	case string(readmodel.BoardPublishProblemNoCheckpoint):
		return &model.NoCheckpointProblem{Message: "board must have at least one checkpoint"}, nil
	case string(readmodel.BoardPublishProblemUnreachableTiles):
		return &model.UnreachableTilesProblem{Message: "board has tiles that cannot be reached from the base", Positions: positions}, nil
	case string(readmodel.BoardPublishProblemUnpairedTiles):
		return &model.UnpairedTilesProblem{Message: "board has warps or bridges without a pair", Positions: positions}, nil
	}
	return nil, fmt.Errorf("unknown board publish problem: %q", kind)
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
		element, err := fromBoardTileElement(tile.Element)
		if err != nil {
			return nil, err
		}
		return model.LandTile{X: tile.X, Y: tile.Y, Element: element}, nil
	case readmodel.BoardTileKindBase:
		return model.BaseTile{X: tile.X, Y: tile.Y}, nil
	case readmodel.BoardTileKindCheckpoint:
		return model.CheckpointTile{X: tile.X, Y: tile.Y}, nil
	case readmodel.BoardTileKindWarp:
		return model.WarpTile{X: tile.X, Y: tile.Y}, nil
	case readmodel.BoardTileKindBridge:
		return model.BridgeTile{X: tile.X, Y: tile.Y}, nil
	case readmodel.BoardTileKindSanctuary:
		return model.SanctuaryTile{X: tile.X, Y: tile.Y}, nil
	case readmodel.BoardTileKindShrine:
		return model.ShrineTile{X: tile.X, Y: tile.Y}, nil
	case readmodel.BoardTileKindProphecyShop:
		return model.ProphecyShopTile{X: tile.X, Y: tile.Y}, nil
	case readmodel.BoardTileKindCardShop:
		return model.CardShopTile{X: tile.X, Y: tile.Y}, nil
	default:
		return nil, fmt.Errorf("graphql: unknown board tile kind: %q", tile.Kind)
	}
}

// @oneOf の「ちょうど 1 つ」は、gqlparser・gqlgen が変数で送られた値を確かめない（0 個・2 個でもここまで届く。結合テストで確かめた）ため、
// ここで確かめ、満たさなければ ok が false（入力の誤り）。GraphQL の errors にしないのは、ほかの入力の誤りと同じく項目ごとに示すため
func toBoardTileInput(tile *model.BoardTileInput) (command.BoardTileInput, bool, error) {
	// 土地以外はどれも x・y だけ
	positions := []struct {
		position *model.TilePositionInput
		kind     readmodel.BoardTileKind
	}{
		{tile.Base, readmodel.BoardTileKindBase},
		{tile.Checkpoint, readmodel.BoardTileKindCheckpoint},
		{tile.Warp, readmodel.BoardTileKindWarp},
		{tile.Bridge, readmodel.BoardTileKindBridge},
		{tile.Sanctuary, readmodel.BoardTileKindSanctuary},
		{tile.Shrine, readmodel.BoardTileKindShrine},
		{tile.ProphecyShop, readmodel.BoardTileKindProphecyShop},
		{tile.CardShop, readmodel.BoardTileKindCardShop},
	}
	written := 0
	if tile.Land != nil {
		written++
	}
	for _, p := range positions {
		if p.position != nil {
			written++
		}
	}
	if written != 1 {
		return command.BoardTileInput{}, false, nil
	}
	if tile.Land != nil {
		element, err := toBoardTileElement(tile.Land.Element)
		if err != nil {
			return command.BoardTileInput{}, false, err
		}
		return command.BoardTileInput{X: tile.Land.X, Y: tile.Land.Y, Kind: string(readmodel.BoardTileKindLand), Element: string(element)}, true, nil
	}
	for _, p := range positions {
		if p.position != nil {
			return command.BoardTileInput{X: p.position.X, Y: p.position.Y, Kind: string(p.kind)}, true, nil
		}
	}
	return command.BoardTileInput{}, false, nil
}

func toBoardTileElement(element model.BoardTileElement) (readmodel.BoardTileElement, error) {
	switch element {
	case model.BoardTileElementFire:
		return readmodel.BoardTileElementFire, nil
	case model.BoardTileElementEarth:
		return readmodel.BoardTileElementEarth, nil
	case model.BoardTileElementWater:
		return readmodel.BoardTileElementWater, nil
	case model.BoardTileElementWind:
		return readmodel.BoardTileElementWind, nil
	case model.BoardTileElementNeutral:
		return readmodel.BoardTileElementNeutral, nil
	case model.BoardTileElementMulti:
		return readmodel.BoardTileElementMulti, nil
	default:
		return "", fmt.Errorf("graphql: unknown board tile element: %s", element)
	}
}

func fromBoardTileElement(element readmodel.BoardTileElement) (model.BoardTileElement, error) {
	switch element {
	case readmodel.BoardTileElementFire:
		return model.BoardTileElementFire, nil
	case readmodel.BoardTileElementEarth:
		return model.BoardTileElementEarth, nil
	case readmodel.BoardTileElementWater:
		return model.BoardTileElementWater, nil
	case readmodel.BoardTileElementWind:
		return model.BoardTileElementWind, nil
	case readmodel.BoardTileElementNeutral:
		return model.BoardTileElementNeutral, nil
	case readmodel.BoardTileElementMulti:
		return model.BoardTileElementMulti, nil
	default:
		return "", fmt.Errorf("graphql: unknown board tile element: %q", element)
	}
}
