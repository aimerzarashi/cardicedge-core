package game_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const boardFields = `id name width height
	defaultRules { targetMagic startingMagic lapBonus checkpointBonus roundLimit diceMin diceMax }
	tileCounts { land base checkpoint fire earth water wind neutral multi warp bridge sanctuary shrine prophecyShop cardShop }
	tiles { __typename x y ... on LandTile { attribute } }
	connections { a { x y } b { x y } }
	pairs { a { x y } b { x y } }`

const boardQuery = `query ($id: UUID!) {
	board(id: $id) {
		__typename
		... on Board { ` + boardFields + ` }
		... on BoardNotFoundError { message }
		... on PlayerNotRegisteredError { message }
	}
}`

const boardsQuery = `query ($first: Int, $after: String) {
	boards(first: $first, after: $after) {
		__typename
		... on BoardsConnection {
			edges { cursor node { ` + boardFields + ` } }
			pageInfo { hasNextPage hasPreviousPage startCursor endCursor }
		}
		... on ValidationErrors {
			errors {
				__typename
				field
				... on FieldOutOfRange { min max }
			}
		}
		... on PlayerNotRegisteredError { message }
	}
}`

func position(x, y int) map[string]any {
	return map[string]any{"x": float64(x), "y": float64(y)}
}

// attribute は土地のときだけ（無ければ空）
func tile(x, y int, typename, attribute string) map[string]any {
	value := map[string]any{"__typename": typename, "x": float64(x), "y": float64(y)}
	if attribute != "" {
		value["attribute"] = attribute
	}
	return value
}

func connection(a, b map[string]any) map[string]any {
	return map[string]any{"a": a, "b": b}
}

// testhelper.PublishBoards が写した公開中のボード
func publishedBoardA() map[string]any {
	return map[string]any{
		"id": master.Boards.PublishedA.String(), "name": "公開中A", "width": float64(2), "height": float64(1),
		"defaultRules": map[string]any{"targetMagic": float64(8000), "startingMagic": float64(600), "lapBonus": float64(300), "checkpointBonus": float64(100), "roundLimit": float64(30), "diceMin": float64(1), "diceMax": float64(6)},
		"tileCounts":   map[string]any{"land": float64(0), "base": float64(1), "checkpoint": float64(1), "fire": float64(0), "earth": float64(0), "water": float64(0), "wind": float64(0), "neutral": float64(0), "multi": float64(0), "warp": float64(0), "bridge": float64(0), "sanctuary": float64(0), "shrine": float64(0), "prophecyShop": float64(0), "cardShop": float64(0)},
		"tiles":        []any{tile(0, 0, "BaseTile", ""), tile(1, 0, "CheckpointTile", "")},
		"connections":  []any{connection(position(0, 0), position(1, 0))},
		"pairs":        []any{},
	}
}

func publishedBoardB() map[string]any {
	return map[string]any{
		"id": master.Boards.PublishedB.String(), "name": "公開中B", "width": float64(3), "height": float64(2),
		"defaultRules": map[string]any{"targetMagic": float64(5000), "startingMagic": float64(0), "lapBonus": float64(200), "checkpointBonus": float64(50), "roundLimit": float64(12), "diceMin": float64(2), "diceMax": float64(8)},
		"tileCounts":   map[string]any{"land": float64(2), "base": float64(1), "checkpoint": float64(1), "fire": float64(1), "earth": float64(0), "water": float64(0), "wind": float64(0), "neutral": float64(0), "multi": float64(1), "warp": float64(2), "bridge": float64(0), "sanctuary": float64(0), "shrine": float64(0), "prophecyShop": float64(0), "cardShop": float64(0)},
		"tiles":        []any{tile(0, 0, "BaseTile", ""), tile(1, 0, "LandTile", "FIRE"), tile(2, 0, "CheckpointTile", ""), tile(0, 1, "WarpTile", ""), tile(1, 1, "LandTile", "MULTI"), tile(2, 1, "WarpTile", "")},
		"connections": []any{
			connection(position(0, 0), position(1, 0)),
			connection(position(1, 0), position(1, 1)),
			connection(position(1, 0), position(2, 0)),
			connection(position(0, 1), position(1, 1)),
		},
		"pairs": []any{connection(position(0, 1), position(2, 1))},
	}
}

func boardNodes(t *testing.T, result map[string]any) []any {
	t.Helper()
	require.Equal(t, "BoardsConnection", result["__typename"])
	nodes := []any{}
	for _, edge := range result["edges"].([]any) {
		nodes = append(nodes, edge.(map[string]any)["node"])
	}
	return nodes
}

// 公開は一度だけ写すので、共有の DB に入っているボードはこの 4 つだけ
func TestBoards_ListsOnlyPublishedBoardsInNameOrder(t *testing.T) {
	t.Parallel()

	result := client.Data(t, registeredPlayerToken(t), boardsQuery, nil, "boards")

	require.Equal(t, []any{publishedBoardA(), publishedBoardB()}, boardNodes(t, result))
	require.Equal(t, false, result["pageInfo"].(map[string]any)["hasNextPage"])
}

func TestBoards_PagesWithCursor(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)

	first := client.Data(t, token, boardsQuery, map[string]any{"first": 1}, "boards")
	after := first["pageInfo"].(map[string]any)["endCursor"]
	second := client.Data(t, token, boardsQuery, map[string]any{"first": 1, "after": after}, "boards")

	require.Equal(t, []any{publishedBoardA()}, boardNodes(t, first))
	require.Equal(t, true, first["pageInfo"].(map[string]any)["hasNextPage"])
	require.Equal(t, []any{publishedBoardB()}, boardNodes(t, second))
	require.Equal(t, false, second["pageInfo"].(map[string]any)["hasNextPage"])
	require.Equal(t, true, second["pageInfo"].(map[string]any)["hasPreviousPage"])
}

func TestBoards_ReturnsValidationErrorsForInvalidPaging(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)

	result := client.Data(t, token, boardsQuery, map[string]any{"first": 101, "after": "not-a-cursor"}, "boards")

	require.Equal(t, "ValidationErrors", result["__typename"])
	require.ElementsMatch(t, []any{
		map[string]any{"__typename": "FieldOutOfRange", "field": "first", "min": float64(1), "max": float64(100)},
		map[string]any{"__typename": "FieldInvalidFormat", "field": "after"},
	}, result["errors"])
}

func TestBoards_ReturnsPlayerNotRegisteredForUnregisteredPlayer(t *testing.T) {
	t.Parallel()

	result := client.Data(t, newPlayerToken(t), boardsQuery, nil, "boards")

	require.Equal(t, "PlayerNotRegisteredError", result["__typename"])
	require.NotEmpty(t, result["message"])
}

func TestBoard_ReturnsPublishedBoard(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)

	b := client.Data(t, token, boardQuery, map[string]any{"id": master.Boards.PublishedB.String()}, "board")

	want := publishedBoardB()
	want["__typename"] = "Board"
	require.Equal(t, want, b)
}

// 準備中・使用不可は写されていても出さない（詳細は、無いものと同じ扱い）
func TestBoard_ReturnsNotFoundForBoardThatIsNotPublished(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		id   uuid.UUID
	}{
		{name: "draft", id: master.Boards.Draft},
		{name: "retired", id: master.Boards.Retired},
		{name: "unknown", id: uuid.New()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := client.Data(t, registeredPlayerToken(t), boardQuery, map[string]any{"id": tt.id.String()}, "board")

			require.Equal(t, "BoardNotFoundError", result["__typename"])
			require.NotEmpty(t, result["message"])
		})
	}
}

func TestBoard_ReturnsPlayerNotRegisteredForUnregisteredPlayer(t *testing.T) {
	t.Parallel()

	result := client.Data(t, newPlayerToken(t), boardQuery, map[string]any{"id": master.Boards.PublishedA.String()}, "board")

	require.Equal(t, "PlayerNotRegisteredError", result["__typename"])
	require.NotEmpty(t, result["message"])
}
