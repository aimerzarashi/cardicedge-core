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

const npcMindsetFields = `id name status revision registeredAt updatedAt
	actionWeights { action weight }
	criterionWeights { criterion weight }`

const npcMindsetQuery = `query ($id: UUID!) {
	npcMindset(id: $id) {
		__typename
		... on NpcMindset { ` + npcMindsetFields + ` }
	}
}`

const npcMindsetsListQuery = `query ($filter: NpcMindsetFilter, $first: Int, $after: String) {
	npcMindsets(filter: $filter, first: $first, after: $after) {
		__typename
		... on NpcMindsetConnection {
			edges { cursor node { ` + npcMindsetFields + ` } }
			pageInfo { hasNextPage }
		}
		... on ValidationErrors { errors { __typename field } }
	}
}`

const npcMindsetErrorFields = `... on ValidationErrors { errors { __typename field ... on FieldTooLong { max } ... on FieldOutOfRange { min max } } }`

// 結果の union に ValidationErrors を含まない操作（削除・公開・使用不可）は、含む操作と同じ断片を書けない
func npcMindsetMutation(name, successFields string, withValidationErrors bool) string {
	errorFields := ""
	if withValidationErrors {
		errorFields = npcMindsetErrorFields
	}
	return `mutation ($input: ` + strings.ToUpper(name[:1]) + name[1:] + `Input!) {
	` + name + `(input: $input) {
		__typename
		... on ` + strings.ToUpper(name[:1]) + name[1:] + `Success { ` + successFields + ` }
		` + errorFields + `
	}
}`
}

var (
	registerNpcMindsetMutation            = npcMindsetMutation("registerNpcMindset", "id revision", true)
	renameNpcMindsetMutation              = npcMindsetMutation("renameNpcMindset", "id revision", true)
	setNpcMindsetActionWeightsMutation    = npcMindsetMutation("setNpcMindsetActionWeights", "id revision", true)
	setNpcMindsetCriterionWeightsMutation = npcMindsetMutation("setNpcMindsetCriterionWeights", "id revision", true)
	deleteNpcMindsetMutation              = npcMindsetMutation("deleteNpcMindset", "id", false)
	publishNpcMindsetMutation             = npcMindsetMutation("publishNpcMindset", "id revision", false)
	retireNpcMindsetMutation              = npcMindsetMutation("retireNpcMindset", "id revision", false)
	setNpcAvatarMindsetsMutation          = npcMindsetMutation("setNpcAvatarMindsets", "id revision", true)
)

// 名前が 20 文字までで、UUID（36 文字）をそのまま付けられないため、先頭の 8 文字を使う
func npcMindsetUniqueID(t *testing.T) string {
	t.Helper()
	return testhelper.UniqueID(t)[:8]
}

func npcMindsetOperation(t *testing.T, token, mutation, field string, input map[string]any) map[string]any {
	t.Helper()
	return client.Data(t, token, mutation, map[string]any{"input": input}, field)
}

func registerNpcMindset(t *testing.T, token, name string) map[string]any {
	t.Helper()
	result := npcMindsetOperation(t, token, registerNpcMindsetMutation, "registerNpcMindset", map[string]any{"name": name})
	require.Equal(t, "RegisterNpcMindsetSuccess", result["__typename"])
	return result
}

func npcMindsetInput(npcMindset map[string]any, extra map[string]any) map[string]any {
	input := map[string]any{"id": npcMindset["id"], "revision": npcMindset["revision"]}
	for k, v := range extra {
		input[k] = v
	}
	return input
}

// 投影されるまで待ち、revision が指定のものになった詳細を返す
func waitNpcMindset(t *testing.T, token string, id any, revision int) map[string]any {
	t.Helper()
	var npcMindset map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		npcMindset, err = client.TryData(t.Context(), token, npcMindsetQuery, map[string]any{"id": id}, "npcMindset")
		if assert.NoError(c, err) {
			assert.Equal(c, "NpcMindset", npcMindset["__typename"])
			assert.EqualValues(c, revision, npcMindset["revision"])
		}
	})
	return npcMindset
}

// 公開中の NPC の思考パターン（公開の結果。投影されるまで待つ）
func publishedNpcMindset(t *testing.T, token, name string) map[string]any {
	t.Helper()
	registered := registerNpcMindset(t, token, name)
	published := npcMindsetOperation(t, token, publishNpcMindsetMutation, "publishNpcMindset", npcMindsetInput(registered, nil))
	require.Equal(t, "PublishNpcMindsetSuccess", published["__typename"])
	waitNpcMindset(t, token, published["id"], 2)
	return published
}

// 種類・基準のすべてを 1 回ずつ並べた重み（指定した 1 つだけ値を変える）
func actionWeightInputs(action string, weight int) []any {
	actions := []string{
		"DRAW_CARD", "DISCARD_CARD", "USE_SPELL", "ROLL_DICE", "CHOOSE_PATH", "PLACE_CREATURE", "INVADE_TERRITORY", "PAY_TOLL", "LEVEL_UP_LAND",
		"EXCHANGE_CREATURE", "CHANGE_LAND_ELEMENT", "MOVE_CREATURE", "ATTACK_WITH_ITEM", "ATTACK_WITHOUT_ITEM", "DEFEND_WITH_ITEM",
		"DEFEND_WITHOUT_ITEM", "USE_CREATURE_AS_ITEM", "RELINQUISH_TERRITORY", "END_TURN",
	}
	inputs := make([]any, 0, len(actions))
	for _, a := range actions {
		value := 1
		if a == action {
			value = weight
		}
		inputs = append(inputs, map[string]any{"action": a, "weight": value})
	}
	return inputs
}

func criterionWeightInputs(criterion string, weight int) []any {
	criteria := []string{"OWN_TOTAL_MAGIC", "RIVAL_TOTAL_MAGIC", "TERRITORY_COUNT"}
	inputs := make([]any, 0, len(criteria))
	for _, c := range criteria {
		value := 1
		if c == criterion {
			value = weight
		}
		inputs = append(inputs, map[string]any{"criterion": c, "weight": value})
	}
	return inputs
}

func TestRegisterNpcMindset_ListsDraftNpcMindsetWithDefaultWeightsOfOne(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := npcMindsetUniqueID(t)

	registered := registerNpcMindset(t, token, " バランス-"+id+" ")

	require.EqualValues(t, 1, registered["revision"])
	var connection map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		connection, err = client.TryData(t.Context(), token, npcMindsetsListQuery, map[string]any{"filter": map[string]any{"nameContains": id}}, "npcMindsets")
		if assert.NoError(c, err) {
			assert.Len(c, connection["edges"], 1)
		}
	})
	node := connection["edges"].([]any)[0].(map[string]any)["node"].(map[string]any)
	require.Equal(t, registered["id"], node["id"])
	require.Equal(t, "バランス-"+id, node["name"])
	require.Equal(t, "DRAFT", node["status"])
	require.Equal(t, node["registeredAt"], node["updatedAt"])
	require.Len(t, node["actionWeights"], 19)
	for _, weight := range node["actionWeights"].([]any) {
		require.EqualValues(t, 1, weight.(map[string]any)["weight"])
	}
	// 採点基準は定義の順（自分の総魔力・相手の総魔力・領地の数）
	require.Equal(t, []any{
		map[string]any{"criterion": "OWN_TOTAL_MAGIC", "weight": float64(1)}, map[string]any{"criterion": "RIVAL_TOTAL_MAGIC", "weight": float64(1)},
		map[string]any{"criterion": "TERRITORY_COUNT", "weight": float64(1)},
	}, node["criterionWeights"])
	detail := waitNpcMindset(t, token, registered["id"], 1)
	delete(detail, "__typename")
	require.Equal(t, node, detail)
}

func TestRegisterNpcMindset_ReturnsFieldErrorForTooLongName(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)

	result := npcMindsetOperation(t, token, registerNpcMindsetMutation, "registerNpcMindset", map[string]any{"name": strings.Repeat("あ", 21)})

	require.Equal(t, "ValidationErrors", result["__typename"])
	require.Equal(t, []any{map[string]any{"__typename": "FieldTooLong", "field": "name", "max": float64(20)}}, result["errors"])
}

func TestNpcMindsets_FiltersByStatusAndRejectsEmptyStatuses(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := npcMindsetUniqueID(t)
	draft := registerNpcMindset(t, token, "下書き-"+id)
	published := publishedNpcMindset(t, token, "公開-"+id)
	waitNpcMindset(t, token, draft["id"], 1)

	filtered := client.Data(t, token, npcMindsetsListQuery, map[string]any{"filter": map[string]any{"nameContains": id, "statuses": []string{"PUBLISHED"}}}, "npcMindsets")
	empty := client.Data(t, token, npcMindsetsListQuery, map[string]any{"filter": map[string]any{"statuses": []string{}}}, "npcMindsets")

	edges := filtered["edges"].([]any)
	require.Len(t, edges, 1)
	require.Equal(t, published["id"], edges[0].(map[string]any)["node"].(map[string]any)["id"])
	require.Equal(t, "ValidationErrors", empty["__typename"])
}

func TestRenameNpcMindset_ShowsNewNameAndFreesOldName(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := npcMindsetUniqueID(t)
	npcMindset := registerNpcMindset(t, token, "バランス-"+id)

	renamed := npcMindsetOperation(t, token, renameNpcMindsetMutation, "renameNpcMindset", npcMindsetInput(npcMindset, map[string]any{"name": "侵略型-" + id}))

	require.Equal(t, "RenameNpcMindsetSuccess", renamed["__typename"])
	require.EqualValues(t, 2, renamed["revision"])
	require.Equal(t, "侵略型-"+id, waitNpcMindset(t, token, npcMindset["id"], 2)["name"])
	registerNpcMindset(t, token, "バランス-"+id)
}

func TestSetNpcMindsetWeights_ShowsReplacedWeightsAndSameWeightsKeepRevision(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	npcMindset := registerNpcMindset(t, token, "侵略型-"+npcMindsetUniqueID(t))

	actions := npcMindsetOperation(t, token, setNpcMindsetActionWeightsMutation, "setNpcMindsetActionWeights",
		npcMindsetInput(npcMindset, map[string]any{"weights": actionWeightInputs("INVADE_TERRITORY", 100)}))
	sameActions := npcMindsetOperation(t, token, setNpcMindsetActionWeightsMutation, "setNpcMindsetActionWeights",
		npcMindsetInput(actions, map[string]any{"weights": actionWeightInputs("INVADE_TERRITORY", 100)}))
	criteria := npcMindsetOperation(t, token, setNpcMindsetCriterionWeightsMutation, "setNpcMindsetCriterionWeights",
		npcMindsetInput(actions, map[string]any{"weights": criterionWeightInputs("RIVAL_TOTAL_MAGIC", 3)}))

	require.Equal(t, "SetNpcMindsetActionWeightsSuccess", actions["__typename"])
	require.EqualValues(t, 2, actions["revision"])
	require.Equal(t, actions, sameActions)
	require.Equal(t, "SetNpcMindsetCriterionWeightsSuccess", criteria["__typename"])
	require.EqualValues(t, 3, criteria["revision"])
	detail := waitNpcMindset(t, token, npcMindset["id"], 3)
	require.Contains(t, detail["actionWeights"], map[string]any{"action": "INVADE_TERRITORY", "weight": float64(100)})
	require.Contains(t, detail["actionWeights"], map[string]any{"action": "PAY_TOLL", "weight": float64(1)})
	require.Contains(t, detail["criterionWeights"], map[string]any{"criterion": "RIVAL_TOTAL_MAGIC", "weight": float64(3)})
}

func TestSetNpcMindsetWeights_ReturnsFieldErrors(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	npcMindset := registerNpcMindset(t, token, "バランス-"+npcMindsetUniqueID(t))
	missing := actionWeightInputs("", 1)[1:]
	duplicated := append(actionWeightInputs("", 1), map[string]any{"action": "END_TURN", "weight": 1})

	outOfRange := npcMindsetOperation(t, token, setNpcMindsetActionWeightsMutation, "setNpcMindsetActionWeights",
		npcMindsetInput(npcMindset, map[string]any{"weights": actionWeightInputs("DRAW_CARD", 101)}))
	missingResult := npcMindsetOperation(t, token, setNpcMindsetActionWeightsMutation, "setNpcMindsetActionWeights",
		npcMindsetInput(npcMindset, map[string]any{"weights": missing}))
	duplicatedResult := npcMindsetOperation(t, token, setNpcMindsetActionWeightsMutation, "setNpcMindsetActionWeights",
		npcMindsetInput(npcMindset, map[string]any{"weights": duplicated}))
	criterionOutOfRange := npcMindsetOperation(t, token, setNpcMindsetCriterionWeightsMutation, "setNpcMindsetCriterionWeights",
		npcMindsetInput(npcMindset, map[string]any{"weights": criterionWeightInputs("TERRITORY_COUNT", -1)}))
	criterionMissing := npcMindsetOperation(t, token, setNpcMindsetCriterionWeightsMutation, "setNpcMindsetCriterionWeights",
		npcMindsetInput(npcMindset, map[string]any{"weights": criterionWeightInputs("", 1)[1:]}))

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldOutOfRange", "field": "weights.0.weight", "min": float64(0), "max": float64(100)},
	}}, outOfRange)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldRequired", "field": "weights"},
	}}, missingResult)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldDuplicated", "field": "weights.19"},
	}}, duplicatedResult)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldOutOfRange", "field": "weights.2.weight", "min": float64(0), "max": float64(100)},
	}}, criterionOutOfRange)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldRequired", "field": "weights"},
	}}, criterionMissing)
}

func TestPublishNpcMindset_PublishesRetiresAndPublishesAgain(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	npcMindset := registerNpcMindset(t, token, "バランス-"+npcMindsetUniqueID(t))

	published := npcMindsetOperation(t, token, publishNpcMindsetMutation, "publishNpcMindset", npcMindsetInput(npcMindset, nil))
	again := npcMindsetOperation(t, token, publishNpcMindsetMutation, "publishNpcMindset", npcMindsetInput(published, nil))
	retired := npcMindsetOperation(t, token, retireNpcMindsetMutation, "retireNpcMindset", npcMindsetInput(published, nil))
	republished := npcMindsetOperation(t, token, publishNpcMindsetMutation, "publishNpcMindset", npcMindsetInput(retired, nil))

	require.EqualValues(t, 2, published["revision"])
	require.Equal(t, published["revision"], again["revision"])
	require.EqualValues(t, 3, retired["revision"])
	require.EqualValues(t, 4, republished["revision"])
	require.Equal(t, "PUBLISHED", waitNpcMindset(t, token, npcMindset["id"], 4)["status"])
}

func TestNpcMindsetOperations_ReturnErrorTypesForNameStatusAndRevision(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := npcMindsetUniqueID(t)
	registerNpcMindset(t, token, "バランス-"+id)
	npcMindset := registerNpcMindset(t, token, "侵略型-"+id)

	used := npcMindsetOperation(t, token, renameNpcMindsetMutation, "renameNpcMindset", npcMindsetInput(npcMindset, map[string]any{"name": " バランス-" + id}))
	retireDraft := npcMindsetOperation(t, token, retireNpcMindsetMutation, "retireNpcMindset", npcMindsetInput(npcMindset, nil))
	stale := npcMindsetOperation(t, token, renameNpcMindsetMutation, "renameNpcMindset", map[string]any{"id": npcMindset["id"], "revision": 7, "name": "湖-" + id})
	published := npcMindsetOperation(t, token, publishNpcMindsetMutation, "publishNpcMindset", npcMindsetInput(npcMindset, nil))
	renamePublished := npcMindsetOperation(t, token, renameNpcMindsetMutation, "renameNpcMindset", npcMindsetInput(published, map[string]any{"name": "湖-" + id}))
	actionsPublished := npcMindsetOperation(t, token, setNpcMindsetActionWeightsMutation, "setNpcMindsetActionWeights", npcMindsetInput(published, map[string]any{"weights": actionWeightInputs("", 1)}))
	criteriaPublished := npcMindsetOperation(t, token, setNpcMindsetCriterionWeightsMutation, "setNpcMindsetCriterionWeights", npcMindsetInput(published, map[string]any{"weights": criterionWeightInputs("", 1)}))
	deletePublished := npcMindsetOperation(t, token, deleteNpcMindsetMutation, "deleteNpcMindset", npcMindsetInput(published, nil))

	require.Equal(t, "NpcMindsetNameAlreadyInUseError", used["__typename"])
	require.Equal(t, "InvalidNpcMindsetStatusTransitionError", retireDraft["__typename"])
	require.Equal(t, "RevisionConflictError", stale["__typename"])
	for _, result := range []map[string]any{renamePublished, actionsPublished, criteriaPublished, deletePublished} {
		require.Equal(t, "NpcMindsetNotDraftError", result["__typename"])
	}
}

func TestDeleteNpcMindset_RemovesDraftNpcMindsetAndFreesName(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	name := "バランス-" + npcMindsetUniqueID(t)
	npcMindset := registerNpcMindset(t, token, name)
	waitNpcMindset(t, token, npcMindset["id"], 1)

	deleted := npcMindsetOperation(t, token, deleteNpcMindsetMutation, "deleteNpcMindset", npcMindsetInput(npcMindset, nil))

	require.Equal(t, "DeleteNpcMindsetSuccess", deleted["__typename"])
	require.Equal(t, npcMindset["id"], deleted["id"])
	registerNpcMindset(t, token, name)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		found, err := client.TryData(t.Context(), token, npcMindsetQuery, map[string]any{"id": npcMindset["id"]}, "npcMindset")
		if assert.NoError(c, err) {
			assert.Equal(c, "NpcMindsetNotFoundError", found["__typename"])
		}
	})
}

func setNpcAvatarMindsets(t *testing.T, token string, npcAvatar map[string]any, mindsets ...any) map[string]any {
	t.Helper()
	// mindsetIds は必須（空でも [] を送る）
	return npcMindsetOperation(t, token, setNpcAvatarMindsetsMutation, "setNpcAvatarMindsets", npcAvatarInput(npcAvatar, map[string]any{"mindsetIds": append([]any{}, mindsets...)}))
}

// 思考パターンはデッキと同じく割り当てた順に並び、全部を置き換える。アバターの公開に割り当ては要らない
func TestSetNpcAvatarMindsets_ShowsMindsetsInGivenOrderAndReplacesThem(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := npcMindsetUniqueID(t)
	npcAvatar := registerNpcAvatar(t, token, "森の番人-"+id)
	first := publishedNpcMindset(t, token, "一つ目-"+id)
	second := publishedNpcMindset(t, token, "二つ目-"+id)

	changed := setNpcAvatarMindsets(t, token, npcAvatar, second["id"], first["id"])
	same := setNpcAvatarMindsets(t, token, changed, first["id"], second["id"])

	require.Equal(t, "SetNpcAvatarMindsetsSuccess", changed["__typename"])
	require.EqualValues(t, 2, changed["revision"])
	require.Equal(t, changed, same)
	var detail map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		detail, err = client.TryData(t.Context(), token, npcAvatarMindsetsQuery, map[string]any{"id": npcAvatar["id"]}, "npcAvatar")
		if assert.NoError(c, err) {
			assert.Len(c, detail["mindsets"], 2)
		}
	})
	require.Equal(t, []any{
		map[string]any{"id": second["id"], "name": "二つ目-" + id, "status": "PUBLISHED"},
		map[string]any{"id": first["id"], "name": "一つ目-" + id, "status": "PUBLISHED"},
	}, detail["mindsets"])
	// デッキの割り当ては変わらない
	require.Empty(t, detail["decks"])
}

const npcAvatarMindsetsQuery = `query ($id: UUID!) {
	npcAvatar(id: $id) {
		... on NpcAvatar { decks { id } mindsets { id name status } }
	}
}`

func TestSetNpcAvatarMindsets_ReturnsFieldErrorPerMindset(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := npcMindsetUniqueID(t)
	npcAvatar := registerNpcAvatar(t, token, "森の番人-"+id)
	published := publishedNpcMindset(t, token, "公開-"+id)
	draft := registerNpcMindset(t, token, "準備中-"+id)

	duplicated := setNpcAvatarMindsets(t, token, npcAvatar, published["id"], published["id"])
	unknown := setNpcAvatarMindsets(t, token, npcAvatar, uuid.NewString())
	notPublished := setNpcAvatarMindsets(t, token, npcAvatar, draft["id"], published["id"])

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldDuplicated", "field": "mindsetIds.1"},
	}}, duplicated)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldNotFound", "field": "mindsetIds.0"},
	}}, unknown)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldUnavailable", "field": "mindsetIds.0"},
	}}, notPublished)
}

func TestNpcMindsetOperations_ReturnResultTypeWhenNotFoundOrNotOperator(t *testing.T) {
	t.Parallel()
	notOperator := key.Sign(t, idptest.ValidClaims("not-operator"))
	unknown := map[string]any{"id": uuid.NewString(), "revision": 1}
	actionWeights := map[string]any{"weights": actionWeightInputs("", 1)}
	criterionWeights := map[string]any{"weights": criterionWeightInputs("", 1)}
	tests := []struct {
		name      string
		token     string
		operation string
		field     string
		variables map[string]any
		want      string
	}{
		{name: "detail_of_unknown", token: operators.Token(t), operation: npcMindsetQuery, field: "npcMindset",
			variables: map[string]any{"id": uuid.NewString()}, want: "NpcMindsetNotFoundError"},
		{name: "rename_unknown", token: operators.Token(t), operation: renameNpcMindsetMutation, field: "renameNpcMindset",
			variables: map[string]any{"input": npcMindsetInput(unknown, map[string]any{"name": "バランス"})}, want: "NpcMindsetNotFoundError"},
		{name: "set_action_weights_unknown", token: operators.Token(t), operation: setNpcMindsetActionWeightsMutation, field: "setNpcMindsetActionWeights",
			variables: map[string]any{"input": npcMindsetInput(unknown, actionWeights)}, want: "NpcMindsetNotFoundError"},
		{name: "set_criterion_weights_unknown", token: operators.Token(t), operation: setNpcMindsetCriterionWeightsMutation, field: "setNpcMindsetCriterionWeights",
			variables: map[string]any{"input": npcMindsetInput(unknown, criterionWeights)}, want: "NpcMindsetNotFoundError"},
		{name: "delete_unknown", token: operators.Token(t), operation: deleteNpcMindsetMutation, field: "deleteNpcMindset",
			variables: map[string]any{"input": unknown}, want: "NpcMindsetNotFoundError"},
		{name: "publish_unknown", token: operators.Token(t), operation: publishNpcMindsetMutation, field: "publishNpcMindset",
			variables: map[string]any{"input": unknown}, want: "NpcMindsetNotFoundError"},
		{name: "retire_unknown", token: operators.Token(t), operation: retireNpcMindsetMutation, field: "retireNpcMindset",
			variables: map[string]any{"input": unknown}, want: "NpcMindsetNotFoundError"},
		{name: "set_avatar_mindsets_unknown", token: operators.Token(t), operation: setNpcAvatarMindsetsMutation, field: "setNpcAvatarMindsets",
			variables: map[string]any{"input": npcAvatarInput(unknown, map[string]any{"mindsetIds": []string{}})}, want: "NpcAvatarNotFoundError"},
		{name: "list_by_not_operator", token: notOperator, operation: npcMindsetsListQuery, field: "npcMindsets",
			variables: map[string]any{}, want: "ForbiddenError"},
		{name: "detail_by_not_operator", token: notOperator, operation: npcMindsetQuery, field: "npcMindset",
			variables: map[string]any{"id": uuid.NewString()}, want: "ForbiddenError"},
		{name: "register_by_not_operator", token: notOperator, operation: registerNpcMindsetMutation, field: "registerNpcMindset",
			variables: map[string]any{"input": map[string]any{"name": "バランス-" + npcMindsetUniqueID(t)}}, want: "ForbiddenError"},
		{name: "set_action_weights_by_not_operator", token: notOperator, operation: setNpcMindsetActionWeightsMutation, field: "setNpcMindsetActionWeights",
			variables: map[string]any{"input": npcMindsetInput(unknown, actionWeights)}, want: "ForbiddenError"},
		{name: "set_avatar_mindsets_by_not_operator", token: notOperator, operation: setNpcAvatarMindsetsMutation, field: "setNpcAvatarMindsets",
			variables: map[string]any{"input": npcAvatarInput(unknown, map[string]any{"mindsetIds": []string{}})}, want: "ForbiddenError"},
		{name: "publish_by_not_operator", token: notOperator, operation: publishNpcMindsetMutation, field: "publishNpcMindset",
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
