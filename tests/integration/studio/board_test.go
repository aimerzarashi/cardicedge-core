package studio_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

const boardFields = `id name status width height revision registeredAt updatedAt
	defaultRules { targetMagic startingMagic lapBonus checkpointBonus }
	tileCounts { land base checkpoint fire earth water wind neutral multi }
	tiles { __typename x y ... on LandTile { attribute } }
	connections { a { x y } b { x y } }`

const boardQuery = `query ($id: UUID!) {
	board(id: $id) {
		__typename
		... on Board { ` + boardFields + ` }
	}
}`

const boardsListQuery = `query ($filter: BoardFilter, $first: Int, $after: String) {
	boards(filter: $filter, first: $first, after: $after) {
		__typename
		... on BoardsConnection {
			edges { cursor node { ` + boardFields + ` } }
			pageInfo { hasNextPage }
		}
		... on ValidationErrors { errors { __typename field } }
	}
}`

const boardErrorFields = `... on ValidationErrors { errors { __typename field ... on FieldOutOfRange { min max } ... on FieldTooSmall { min } ... on FieldNotGreaterThan { than } } }`

const registerBoardMutation = `mutation ($input: RegisterBoardInput!) {
	registerBoard(input: $input) {
		__typename
		... on RegisterBoardSuccess { id revision }
		` + boardErrorFields + `
	}
}`

const renameBoardMutation = `mutation ($input: RenameBoardInput!) {
	renameBoard(input: $input) {
		__typename
		... on RenameBoardSuccess { id revision }
		` + boardErrorFields + `
	}
}`

const changeBoardLayoutMutation = `mutation ($input: ChangeBoardLayoutInput!) {
	changeBoardLayout(input: $input) {
		__typename
		... on ChangeBoardLayoutSuccess { id revision }
		` + boardErrorFields + `
	}
}`

const changeBoardDefaultRulesMutation = `mutation ($input: ChangeBoardDefaultRulesInput!) {
	changeBoardDefaultRules(input: $input) {
		__typename
		... on ChangeBoardDefaultRulesSuccess { id revision }
		` + boardErrorFields + `
	}
}`

const deleteBoardMutation = `mutation ($input: DeleteBoardInput!) {
	deleteBoard(input: $input) {
		__typename
		... on DeleteBoardSuccess { id }
	}
}`

const publishBoardMutation = `mutation ($input: PublishBoardInput!) {
	publishBoard(input: $input) {
		__typename
		... on PublishBoardSuccess { id revision }
		... on BoardNotPublishableError { baseCount checkpointCount unreachableTiles { x y } }
	}
}`

const retireBoardMutation = `mutation ($input: RetireBoardInput!) {
	retireBoard(input: $input) {
		__typename
		... on RetireBoardSuccess { id revision }
	}
}`

func boardOperation(t *testing.T, token, mutation, field string, input map[string]any) map[string]any {
	t.Helper()
	return client.Data(t, token, mutation, map[string]any{"input": input}, field)
}

func registerBoard(t *testing.T, token, name string) map[string]any {
	t.Helper()
	result := boardOperation(t, token, registerBoardMutation, "registerBoard", map[string]any{"name": name})
	require.Equal(t, "RegisterBoardSuccess", result["__typename"])
	return result
}

func boardInput(board map[string]any, extra map[string]any) map[string]any {
	input := map[string]any{"id": board["id"], "revision": board["revision"]}
	for k, v := range extra {
		input[k] = v
	}
	return input
}

// 土地には火属性を付ける（属性を確かめないテストで使う）
func boardTile(x, y int, kind string) map[string]any {
	if kind == "LAND" {
		return boardTileWithAttribute(x, y, kind, "FIRE")
	}
	return map[string]any{strings.ToLower(kind): map[string]any{"x": x, "y": y}}
}

func boardTileWithAttribute(x, y int, kind, attribute string) map[string]any {
	return map[string]any{strings.ToLower(kind): map[string]any{"x": x, "y": y, "attribute": attribute}}
}

func boardConnection(ax, ay, bx, by int) map[string]any {
	return map[string]any{"a": map[string]any{"x": ax, "y": ay}, "b": map[string]any{"x": bx, "y": by}}
}

func changeBoardLayout(t *testing.T, token string, board map[string]any, width, height int, tiles, connections []map[string]any) map[string]any {
	t.Helper()
	// nil のスライスは JSON の null になり、必須のリストの入力の誤りになるため、空のリストにする
	if tiles == nil {
		tiles = []map[string]any{}
	}
	if connections == nil {
		connections = []map[string]any{}
	}
	return boardOperation(t, token, changeBoardLayoutMutation, "changeBoardLayout",
		boardInput(board, map[string]any{"width": width, "height": height, "tiles": tiles, "connections": connections}))
}

// 拠点 → 土地 → チェックポイントが横に並ぶ、公開できる 3×3 を入れる
func changePublishableLayout(t *testing.T, token string, board map[string]any) map[string]any {
	t.Helper()
	return changeBoardLayout(t, token, board, 3, 3,
		[]map[string]any{boardTile(0, 0, "BASE"), boardTile(1, 0, "LAND"), boardTile(2, 0, "CHECKPOINT")},
		[]map[string]any{boardConnection(0, 0, 1, 0), boardConnection(1, 0, 2, 0)})
}

// 投影されるまで待ち、revision が指定のものになった詳細を返す
func waitBoard(t *testing.T, token string, id any, revision int) map[string]any {
	t.Helper()
	var board map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		board, err = client.TryData(t.Context(), token, boardQuery, map[string]any{"id": id}, "board")
		if assert.NoError(c, err) {
			assert.Equal(c, "Board", board["__typename"])
			assert.EqualValues(c, revision, board["revision"])
		}
	})
	return board
}

func TestRegisterBoard_ListsDraftBoardOfDefaultSize(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)

	registered := registerBoard(t, token, " 草原-"+id+" ")

	require.EqualValues(t, 1, registered["revision"])
	var connection map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		connection, err = client.TryData(t.Context(), token, boardsListQuery, map[string]any{"filter": map[string]any{"nameContains": id}}, "boards")
		if assert.NoError(c, err) {
			assert.Len(c, connection["edges"], 1)
		}
	})
	node := connection["edges"].([]any)[0].(map[string]any)["node"].(map[string]any)
	require.Equal(t, registered["id"], node["id"])
	require.Equal(t, "草原-"+id, node["name"])
	require.Equal(t, "DRAFT", node["status"])
	require.EqualValues(t, 8, node["width"])
	require.EqualValues(t, 8, node["height"])
	require.Empty(t, node["tiles"])
	require.Empty(t, node["connections"])
	require.Equal(t, map[string]any{"targetMagic": float64(8000), "startingMagic": float64(600), "lapBonus": float64(300), "checkpointBonus": float64(100)}, node["defaultRules"])
	require.Equal(t, map[string]any{"land": float64(0), "base": float64(0), "checkpoint": float64(0), "fire": float64(0), "earth": float64(0), "water": float64(0), "wind": float64(0), "neutral": float64(0), "multi": float64(0)}, node["tileCounts"])
	require.Equal(t, node["registeredAt"], node["updatedAt"])
	detail := waitBoard(t, token, registered["id"], 1)
	delete(detail, "__typename")
	require.Equal(t, node, detail)
}

func TestBoards_FiltersByStatusAndRejectsEmptyStatuses(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	draft := registerBoard(t, token, "下書き-"+id)
	ready := registerBoard(t, token, "完成-"+id)
	changed := changePublishableLayout(t, token, ready)
	published := boardOperation(t, token, publishBoardMutation, "publishBoard", boardInput(changed, nil))
	require.Equal(t, "PublishBoardSuccess", published["__typename"])
	waitBoard(t, token, published["id"], 3)
	waitBoard(t, token, draft["id"], 1)

	filtered := client.Data(t, token, boardsListQuery, map[string]any{"filter": map[string]any{"nameContains": id, "statuses": []string{"PUBLISHED"}}}, "boards")
	empty := client.Data(t, token, boardsListQuery, map[string]any{"filter": map[string]any{"statuses": []string{}}}, "boards")

	edges := filtered["edges"].([]any)
	require.Len(t, edges, 1)
	require.Equal(t, ready["id"], edges[0].(map[string]any)["node"].(map[string]any)["id"])
	require.Equal(t, "ValidationErrors", empty["__typename"])
}

func TestChangeBoardLayout_ShowsWholeLayoutAndReplacesIt(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	board := registerBoard(t, token, "草原-"+testhelper.UniqueID(t))

	changed := changeBoardLayout(t, token, board, 3, 2,
		[]map[string]any{boardTileWithAttribute(2, 1, "LAND", "MULTI"), boardTile(0, 0, "BASE"), boardTile(1, 0, "CHECKPOINT")},
		[]map[string]any{boardConnection(1, 0, 0, 0)})

	require.Equal(t, "ChangeBoardLayoutSuccess", changed["__typename"])
	require.EqualValues(t, 2, changed["revision"])
	detail := waitBoard(t, token, changed["id"], 2)
	require.EqualValues(t, 3, detail["width"])
	require.EqualValues(t, 2, detail["height"])
	require.Equal(t, []any{
		map[string]any{"__typename": "BaseTile", "x": float64(0), "y": float64(0)},
		map[string]any{"__typename": "CheckpointTile", "x": float64(1), "y": float64(0)},
		map[string]any{"__typename": "LandTile", "x": float64(2), "y": float64(1), "attribute": "MULTI"},
	}, detail["tiles"])
	require.Equal(t, []any{map[string]any{
		"a": map[string]any{"x": float64(0), "y": float64(0)}, "b": map[string]any{"x": float64(1), "y": float64(0)},
	}}, detail["connections"])
	require.Equal(t, map[string]any{"land": float64(1), "base": float64(1), "checkpoint": float64(1), "fire": float64(0), "earth": float64(0), "water": float64(0), "wind": float64(0), "neutral": float64(0), "multi": float64(1)}, detail["tileCounts"])

	replaced := changeBoardLayout(t, token, ref(changed), 1, 1, nil, nil)

	require.EqualValues(t, 3, replaced["revision"])
	detail = waitBoard(t, token, replaced["id"], 3)
	require.Empty(t, detail["tiles"])
	require.Empty(t, detail["connections"])
	require.Equal(t, map[string]any{"land": float64(0), "base": float64(0), "checkpoint": float64(0), "fire": float64(0), "earth": float64(0), "water": float64(0), "wind": float64(0), "neutral": float64(0), "multi": float64(0)}, detail["tileCounts"])
	same := changeBoardLayout(t, token, ref(replaced), 1, 1, nil, nil)
	require.EqualValues(t, 3, same["revision"])
}

func TestChangeBoardLayout_ReturnsFieldErrorPerViolation(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	board := registerBoard(t, token, "草原-"+testhelper.UniqueID(t))

	rangeErrors := changeBoardLayout(t, token, board, 0, 33, []map[string]any{boardTile(99, 99, "LAND")}, nil)
	layoutErrors := changeBoardLayout(t, token, board, 3, 2,
		[]map[string]any{boardTile(0, 0, "LAND"), boardTile(3, 2, "LAND"), boardTile(0, 0, "BASE"), boardTile(1, 1, "LAND")},
		[]map[string]any{boardConnection(0, 0, 1, 1), boardConnection(0, 0, 0, 1), boardConnection(1, 1, 1, 0), boardConnection(0, 1, 0, 0)})

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldOutOfRange", "field": "width", "min": float64(1), "max": float64(32)},
		map[string]any{"__typename": "FieldOutOfRange", "field": "height", "min": float64(1), "max": float64(32)},
	}}, rangeErrors)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldOutOfRange", "field": "tiles.1.land.x", "min": float64(0), "max": float64(2)},
		map[string]any{"__typename": "FieldOutOfRange", "field": "tiles.1.land.y", "min": float64(0), "max": float64(1)},
		map[string]any{"__typename": "FieldDuplicated", "field": "tiles.2"},
		map[string]any{"__typename": "FieldNotAdjacent", "field": "connections.0"},
		map[string]any{"__typename": "FieldNotFound", "field": "connections.1.b"},
		map[string]any{"__typename": "FieldNotFound", "field": "connections.2.b"},
		map[string]any{"__typename": "FieldNotFound", "field": "connections.3.a"},
		map[string]any{"__typename": "FieldDuplicated", "field": "connections.3"},
	}}, layoutErrors)
}

func TestChangeBoardLayout_ReturnsFieldErrorWithWrittenKindInPath(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	board := registerBoard(t, token, "草原-"+testhelper.UniqueID(t))

	invalid := changeBoardLayout(t, token, board, 2, 1,
		[]map[string]any{boardTile(2, 0, "BASE"), boardTile(0, 1, "CHECKPOINT")}, nil)

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldOutOfRange", "field": "tiles.0.base.x", "min": float64(0), "max": float64(1)},
		map[string]any{"__typename": "FieldOutOfRange", "field": "tiles.1.checkpoint.y", "min": float64(0), "max": float64(0)},
	}}, invalid)
}

// @oneOf の「ちょうど 1 つ」は、変数で送られた値だと GraphQL の層（gqlparser・gqlgen）が確かめず、presentation 層で確かめる
func TestChangeBoardLayout_ReturnsFieldErrorForTileWithoutExactlyOneKind(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	board := registerBoard(t, token, "草原-"+testhelper.UniqueID(t))
	position := map[string]any{"x": 0, "y": 0}

	invalid := changeBoardLayout(t, token, board, 3, 1,
		[]map[string]any{{}, {"base": position, "checkpoint": position}, boardTile(2, 0, "LAND")}, nil)

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldInvalidFormat", "field": "tiles.0"},
		map[string]any{"__typename": "FieldInvalidFormat", "field": "tiles.1"},
	}}, invalid)
}

func defaultRulesInput(targetMagic, startingMagic, lapBonus, checkpointBonus int) map[string]any {
	return map[string]any{"targetMagic": targetMagic, "startingMagic": startingMagic, "lapBonus": lapBonus, "checkpointBonus": checkpointBonus}
}

func TestChangeBoardDefaultRules_ShowsNewValuesAndKeepsLayout(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	board := registerBoard(t, token, "草原-"+testhelper.UniqueID(t))
	withLayout := changePublishableLayout(t, token, board)

	changed := boardOperation(t, token, changeBoardDefaultRulesMutation, "changeBoardDefaultRules", boardInput(withLayout, defaultRulesInput(5000, 0, 200, 50)))

	require.Equal(t, "ChangeBoardDefaultRulesSuccess", changed["__typename"])
	require.EqualValues(t, 3, changed["revision"])
	detail := waitBoard(t, token, changed["id"], 3)
	require.Equal(t, map[string]any{"targetMagic": float64(5000), "startingMagic": float64(0), "lapBonus": float64(200), "checkpointBonus": float64(50)}, detail["defaultRules"])
	require.Len(t, detail["tiles"], 3)
	same := boardOperation(t, token, changeBoardDefaultRulesMutation, "changeBoardDefaultRules", boardInput(ref(changed), defaultRulesInput(5000, 0, 200, 50)))
	require.EqualValues(t, 3, same["revision"])
}

func TestChangeBoardDefaultRules_ReturnsFieldErrorPerViolation(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	board := registerBoard(t, token, "草原-"+testhelper.UniqueID(t))

	tooSmall := boardOperation(t, token, changeBoardDefaultRulesMutation, "changeBoardDefaultRules", boardInput(board, defaultRulesInput(0, -1, -2, -3)))
	notGreater := boardOperation(t, token, changeBoardDefaultRulesMutation, "changeBoardDefaultRules", boardInput(board, defaultRulesInput(600, 600, 0, 0)))

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldTooSmall", "field": "targetMagic", "min": float64(1)},
		map[string]any{"__typename": "FieldTooSmall", "field": "startingMagic", "min": float64(0)},
		map[string]any{"__typename": "FieldTooSmall", "field": "lapBonus", "min": float64(0)},
		map[string]any{"__typename": "FieldTooSmall", "field": "checkpointBonus", "min": float64(0)},
	}}, tooSmall)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldNotGreaterThan", "field": "targetMagic", "than": "startingMagic"},
	}}, notGreater)
}

func TestPublishBoard_ReturnsConditionsWhenNotPublishable(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	board := registerBoard(t, token, "草原-"+testhelper.UniqueID(t))
	empty := boardOperation(t, token, publishBoardMutation, "publishBoard", boardInput(board, nil))
	changed := changeBoardLayout(t, token, board, 3, 3,
		[]map[string]any{boardTile(0, 0, "BASE"), boardTile(1, 0, "CHECKPOINT"), boardTile(2, 2, "LAND"), boardTile(0, 2, "LAND")},
		[]map[string]any{boardConnection(0, 0, 1, 0)})

	unreachable := boardOperation(t, token, publishBoardMutation, "publishBoard", boardInput(changed, nil))

	require.Equal(t, map[string]any{
		"__typename": "BoardNotPublishableError", "baseCount": float64(0), "checkpointCount": float64(0), "unreachableTiles": []any{},
	}, empty)
	require.Equal(t, map[string]any{
		"__typename": "BoardNotPublishableError", "baseCount": float64(1), "checkpointCount": float64(1),
		"unreachableTiles": []any{
			map[string]any{"x": float64(0), "y": float64(2)},
			map[string]any{"x": float64(2), "y": float64(2)},
		},
	}, unreachable)
}

func TestPublishBoard_PublishesRetiresAndPublishesAgain(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	board := registerBoard(t, token, "草原-"+testhelper.UniqueID(t))
	changed := changePublishableLayout(t, token, board)

	published := boardOperation(t, token, publishBoardMutation, "publishBoard", boardInput(changed, nil))
	again := boardOperation(t, token, publishBoardMutation, "publishBoard", boardInput(published, nil))
	retired := boardOperation(t, token, retireBoardMutation, "retireBoard", boardInput(published, nil))
	republished := boardOperation(t, token, publishBoardMutation, "publishBoard", boardInput(retired, nil))

	require.EqualValues(t, 3, published["revision"])
	require.Equal(t, published["revision"], again["revision"])
	require.EqualValues(t, 4, retired["revision"])
	require.EqualValues(t, 5, republished["revision"])
	require.Equal(t, "PUBLISHED", waitBoard(t, token, board["id"], 5)["status"])
}

func TestBoardOperations_ReturnErrorTypesForNameStatusAndRevision(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	registerBoard(t, token, "草原-"+id)
	board := registerBoard(t, token, "砂漠-"+id)

	used := boardOperation(t, token, renameBoardMutation, "renameBoard", boardInput(board, map[string]any{"name": " 草原-" + id}))
	retireDraft := boardOperation(t, token, retireBoardMutation, "retireBoard", boardInput(board, nil))
	stale := boardOperation(t, token, renameBoardMutation, "renameBoard", map[string]any{"id": board["id"], "revision": 7, "name": "湖-" + id})
	changed := changePublishableLayout(t, token, board)
	published := boardOperation(t, token, publishBoardMutation, "publishBoard", boardInput(changed, nil))
	renamePublished := boardOperation(t, token, renameBoardMutation, "renameBoard", boardInput(published, map[string]any{"name": "湖-" + id}))
	layoutPublished := changeBoardLayout(t, token, published, 1, 1, nil, nil)
	rulesPublished := boardOperation(t, token, changeBoardDefaultRulesMutation, "changeBoardDefaultRules", boardInput(published, defaultRulesInput(5000, 0, 200, 50)))
	deletePublished := boardOperation(t, token, deleteBoardMutation, "deleteBoard", boardInput(published, nil))

	require.Equal(t, "BoardNameAlreadyInUseError", used["__typename"])
	require.Equal(t, "InvalidBoardStatusTransitionError", retireDraft["__typename"])
	require.Equal(t, "RevisionConflictError", stale["__typename"])
	require.Equal(t, "BoardNotDraftError", renamePublished["__typename"])
	require.Equal(t, "BoardNotDraftError", layoutPublished["__typename"])
	require.Equal(t, "BoardNotDraftError", rulesPublished["__typename"])
	require.Equal(t, "BoardNotDraftError", deletePublished["__typename"])
}

func TestDeleteBoard_RemovesDraftBoardAndFreesName(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	name := "草原-" + testhelper.UniqueID(t)
	board := registerBoard(t, token, name)
	waitBoard(t, token, board["id"], 1)

	deleted := boardOperation(t, token, deleteBoardMutation, "deleteBoard", boardInput(board, nil))

	require.Equal(t, "DeleteBoardSuccess", deleted["__typename"])
	require.Equal(t, board["id"], deleted["id"])
	registerBoard(t, token, name)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		found, err := client.TryData(t.Context(), token, boardQuery, map[string]any{"id": board["id"]}, "board")
		if assert.NoError(c, err) {
			assert.Equal(c, "BoardNotFoundError", found["__typename"])
		}
	})
}

func TestBoardOperations_ReturnResultTypeWhenNotFoundOrNotOperator(t *testing.T) {
	t.Parallel()
	notOperator := key.Sign(t, idptest.ValidClaims("not-operator"))
	unknown := map[string]any{"id": uuid.NewString(), "revision": 1}
	rules := boardInput(unknown, defaultRulesInput(5000, 0, 200, 50))
	layout := boardInput(unknown, map[string]any{"width": 1, "height": 1, "tiles": []any{}, "connections": []any{}})
	tests := []struct {
		name      string
		token     string
		operation string
		field     string
		variables map[string]any
		want      string
	}{
		{name: "detail_of_unknown", token: operators.Token(t), operation: boardQuery, field: "board",
			variables: map[string]any{"id": uuid.NewString()}, want: "BoardNotFoundError"},
		{name: "rename_unknown", token: operators.Token(t), operation: renameBoardMutation, field: "renameBoard",
			variables: map[string]any{"input": boardInput(unknown, map[string]any{"name": "草原"})}, want: "BoardNotFoundError"},
		{name: "change_layout_unknown", token: operators.Token(t), operation: changeBoardLayoutMutation, field: "changeBoardLayout",
			variables: map[string]any{"input": layout}, want: "BoardNotFoundError"},
		{name: "change_default_rules_unknown", token: operators.Token(t), operation: changeBoardDefaultRulesMutation, field: "changeBoardDefaultRules",
			variables: map[string]any{"input": rules}, want: "BoardNotFoundError"},
		{name: "delete_unknown", token: operators.Token(t), operation: deleteBoardMutation, field: "deleteBoard",
			variables: map[string]any{"input": unknown}, want: "BoardNotFoundError"},
		{name: "publish_unknown", token: operators.Token(t), operation: publishBoardMutation, field: "publishBoard",
			variables: map[string]any{"input": unknown}, want: "BoardNotFoundError"},
		{name: "retire_unknown", token: operators.Token(t), operation: retireBoardMutation, field: "retireBoard",
			variables: map[string]any{"input": unknown}, want: "BoardNotFoundError"},
		{name: "list_by_not_operator", token: notOperator, operation: boardsListQuery, field: "boards",
			variables: map[string]any{}, want: "ForbiddenError"},
		{name: "detail_by_not_operator", token: notOperator, operation: boardQuery, field: "board",
			variables: map[string]any{"id": uuid.NewString()}, want: "ForbiddenError"},
		{name: "register_by_not_operator", token: notOperator, operation: registerBoardMutation, field: "registerBoard",
			variables: map[string]any{"input": map[string]any{"name": "草原-" + testhelper.UniqueID(t)}}, want: "ForbiddenError"},
		{name: "rename_by_not_operator", token: notOperator, operation: renameBoardMutation, field: "renameBoard",
			variables: map[string]any{"input": boardInput(unknown, map[string]any{"name": "草原"})}, want: "ForbiddenError"},
		{name: "change_layout_by_not_operator", token: notOperator, operation: changeBoardLayoutMutation, field: "changeBoardLayout",
			variables: map[string]any{"input": layout}, want: "ForbiddenError"},
		{name: "change_default_rules_by_not_operator", token: notOperator, operation: changeBoardDefaultRulesMutation, field: "changeBoardDefaultRules",
			variables: map[string]any{"input": rules}, want: "ForbiddenError"},
		{name: "delete_by_not_operator", token: notOperator, operation: deleteBoardMutation, field: "deleteBoard",
			variables: map[string]any{"input": unknown}, want: "ForbiddenError"},
		{name: "publish_by_not_operator", token: notOperator, operation: publishBoardMutation, field: "publishBoard",
			variables: map[string]any{"input": unknown}, want: "ForbiddenError"},
		{name: "retire_by_not_operator", token: notOperator, operation: retireBoardMutation, field: "retireBoard",
			variables: map[string]any{"input": unknown}, want: "ForbiddenError"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := client.Post(t, tt.token, tt.operation, tt.variables)

			require.Equal(t, http.StatusOK, status)
			require.Nil(t, body["errors"])
			require.Equal(t, tt.want, body["data"].(map[string]any)[tt.field].(map[string]any)["__typename"])
		})
	}
}
