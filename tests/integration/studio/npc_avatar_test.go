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

const npcAvatarFields = `id name status revision registeredAt updatedAt`

const npcAvatarQuery = `query ($id: UUID!) {
	npcAvatar(id: $id) {
		__typename
		... on NpcAvatar { ` + npcAvatarFields + ` }
	}
}`

const npcAvatarsListQuery = `query ($filter: NpcAvatarFilter, $first: Int, $after: String) {
	npcAvatars(filter: $filter, first: $first, after: $after) {
		__typename
		... on NpcAvatarConnection {
			edges { cursor node { ` + npcAvatarFields + ` } }
			pageInfo { hasNextPage }
		}
		... on ValidationErrors { errors { __typename field } }
	}
}`

const npcAvatarErrorFields = `... on ValidationErrors { errors { __typename field ... on FieldTooLong { max } } }`

const registerNpcAvatarMutation = `mutation ($input: RegisterNpcAvatarInput!) {
	registerNpcAvatar(input: $input) {
		__typename
		... on RegisterNpcAvatarSuccess { id revision }
		` + npcAvatarErrorFields + `
	}
}`

const renameNpcAvatarMutation = `mutation ($input: RenameNpcAvatarInput!) {
	renameNpcAvatar(input: $input) {
		__typename
		... on RenameNpcAvatarSuccess { id revision }
		` + npcAvatarErrorFields + `
	}
}`

const deleteNpcAvatarMutation = `mutation ($input: DeleteNpcAvatarInput!) {
	deleteNpcAvatar(input: $input) {
		__typename
		... on DeleteNpcAvatarSuccess { id }
	}
}`

const publishNpcAvatarMutation = `mutation ($input: PublishNpcAvatarInput!) {
	publishNpcAvatar(input: $input) {
		__typename
		... on PublishNpcAvatarSuccess { id revision }
	}
}`

const retireNpcAvatarMutation = `mutation ($input: RetireNpcAvatarInput!) {
	retireNpcAvatar(input: $input) {
		__typename
		... on RetireNpcAvatarSuccess { id revision }
	}
}`

// 名前が 20 文字までで、UUID（36 文字）をそのまま付けられないため、先頭の 8 文字を使う
func npcAvatarUniqueID(t *testing.T) string {
	t.Helper()
	return testhelper.UniqueID(t)[:8]
}

func npcAvatarOperation(t *testing.T, token, mutation, field string, input map[string]any) map[string]any {
	t.Helper()
	return client.Data(t, token, mutation, map[string]any{"input": input}, field)
}

func registerNpcAvatar(t *testing.T, token, name string) map[string]any {
	t.Helper()
	result := npcAvatarOperation(t, token, registerNpcAvatarMutation, "registerNpcAvatar", map[string]any{"name": name})
	require.Equal(t, "RegisterNpcAvatarSuccess", result["__typename"])
	return result
}

func npcAvatarInput(npcAvatar map[string]any, extra map[string]any) map[string]any {
	input := map[string]any{"id": npcAvatar["id"], "revision": npcAvatar["revision"]}
	for k, v := range extra {
		input[k] = v
	}
	return input
}

// 投影されるまで待ち、revision が指定のものになった詳細を返す
func waitNpcAvatar(t *testing.T, token string, id any, revision int) map[string]any {
	t.Helper()
	var npcAvatar map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		npcAvatar, err = client.TryData(t.Context(), token, npcAvatarQuery, map[string]any{"id": id}, "npcAvatar")
		if assert.NoError(c, err) {
			assert.Equal(c, "NpcAvatar", npcAvatar["__typename"])
			assert.EqualValues(c, revision, npcAvatar["revision"])
		}
	})
	return npcAvatar
}

func TestRegisterNpcAvatar_ListsDraftNpcAvatar(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := npcAvatarUniqueID(t)

	registered := registerNpcAvatar(t, token, " 森の番人-"+id+" ")

	require.EqualValues(t, 1, registered["revision"])
	var connection map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		connection, err = client.TryData(t.Context(), token, npcAvatarsListQuery, map[string]any{"filter": map[string]any{"nameContains": id}}, "npcAvatars")
		if assert.NoError(c, err) {
			assert.Len(c, connection["edges"], 1)
		}
	})
	node := connection["edges"].([]any)[0].(map[string]any)["node"].(map[string]any)
	require.Equal(t, registered["id"], node["id"])
	require.Equal(t, "森の番人-"+id, node["name"])
	require.Equal(t, "DRAFT", node["status"])
	require.Equal(t, node["registeredAt"], node["updatedAt"])
	detail := waitNpcAvatar(t, token, registered["id"], 1)
	delete(detail, "__typename")
	require.Equal(t, node, detail)
}

func TestRegisterNpcAvatar_ReturnsFieldErrorForTooLongName(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)

	result := npcAvatarOperation(t, token, registerNpcAvatarMutation, "registerNpcAvatar", map[string]any{"name": strings.Repeat("あ", 21)})

	require.Equal(t, "ValidationErrors", result["__typename"])
	require.Equal(t, []any{map[string]any{"__typename": "FieldTooLong", "field": "name", "max": float64(20)}}, result["errors"])
}

func TestNpcAvatars_FiltersByStatusAndRejectsEmptyStatuses(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := npcAvatarUniqueID(t)
	draft := registerNpcAvatar(t, token, "下書き-"+id)
	ready := registerNpcAvatar(t, token, "完成-"+id)
	published := npcAvatarOperation(t, token, publishNpcAvatarMutation, "publishNpcAvatar", npcAvatarInput(ready, nil))
	require.Equal(t, "PublishNpcAvatarSuccess", published["__typename"])
	waitNpcAvatar(t, token, published["id"], 2)
	waitNpcAvatar(t, token, draft["id"], 1)

	filtered := client.Data(t, token, npcAvatarsListQuery, map[string]any{"filter": map[string]any{"nameContains": id, "statuses": []string{"PUBLISHED"}}}, "npcAvatars")
	empty := client.Data(t, token, npcAvatarsListQuery, map[string]any{"filter": map[string]any{"statuses": []string{}}}, "npcAvatars")

	edges := filtered["edges"].([]any)
	require.Len(t, edges, 1)
	require.Equal(t, ready["id"], edges[0].(map[string]any)["node"].(map[string]any)["id"])
	require.Equal(t, "ValidationErrors", empty["__typename"])
}

func TestRenameNpcAvatar_ShowsNewNameAndFreesOldName(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := npcAvatarUniqueID(t)
	npcAvatar := registerNpcAvatar(t, token, "森の番人-"+id)

	renamed := npcAvatarOperation(t, token, renameNpcAvatarMutation, "renameNpcAvatar", npcAvatarInput(npcAvatar, map[string]any{"name": "炎の剣士-" + id}))

	require.Equal(t, "RenameNpcAvatarSuccess", renamed["__typename"])
	require.EqualValues(t, 2, renamed["revision"])
	require.Equal(t, "炎の剣士-"+id, waitNpcAvatar(t, token, npcAvatar["id"], 2)["name"])
	registerNpcAvatar(t, token, "森の番人-"+id)
}

func TestPublishNpcAvatar_PublishesRetiresAndPublishesAgain(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	npcAvatar := registerNpcAvatar(t, token, "森の番人-"+npcAvatarUniqueID(t))

	published := npcAvatarOperation(t, token, publishNpcAvatarMutation, "publishNpcAvatar", npcAvatarInput(npcAvatar, nil))
	again := npcAvatarOperation(t, token, publishNpcAvatarMutation, "publishNpcAvatar", npcAvatarInput(published, nil))
	retired := npcAvatarOperation(t, token, retireNpcAvatarMutation, "retireNpcAvatar", npcAvatarInput(published, nil))
	republished := npcAvatarOperation(t, token, publishNpcAvatarMutation, "publishNpcAvatar", npcAvatarInput(retired, nil))

	require.EqualValues(t, 2, published["revision"])
	require.Equal(t, published["revision"], again["revision"])
	require.EqualValues(t, 3, retired["revision"])
	require.EqualValues(t, 4, republished["revision"])
	require.Equal(t, "PUBLISHED", waitNpcAvatar(t, token, npcAvatar["id"], 4)["status"])
}

func TestNpcAvatarOperations_ReturnErrorTypesForNameStatusAndRevision(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := npcAvatarUniqueID(t)
	registerNpcAvatar(t, token, "森の番人-"+id)
	npcAvatar := registerNpcAvatar(t, token, "炎の剣士-"+id)

	used := npcAvatarOperation(t, token, renameNpcAvatarMutation, "renameNpcAvatar", npcAvatarInput(npcAvatar, map[string]any{"name": " 森の番人-" + id}))
	retireDraft := npcAvatarOperation(t, token, retireNpcAvatarMutation, "retireNpcAvatar", npcAvatarInput(npcAvatar, nil))
	stale := npcAvatarOperation(t, token, renameNpcAvatarMutation, "renameNpcAvatar", map[string]any{"id": npcAvatar["id"], "revision": 7, "name": "湖-" + id})
	published := npcAvatarOperation(t, token, publishNpcAvatarMutation, "publishNpcAvatar", npcAvatarInput(npcAvatar, nil))
	renamePublished := npcAvatarOperation(t, token, renameNpcAvatarMutation, "renameNpcAvatar", npcAvatarInput(published, map[string]any{"name": "湖-" + id}))
	deletePublished := npcAvatarOperation(t, token, deleteNpcAvatarMutation, "deleteNpcAvatar", npcAvatarInput(published, nil))

	require.Equal(t, "NpcAvatarNameAlreadyInUseError", used["__typename"])
	require.Equal(t, "InvalidNpcAvatarStatusTransitionError", retireDraft["__typename"])
	require.Equal(t, "RevisionConflictError", stale["__typename"])
	require.Equal(t, "NpcAvatarNotDraftError", renamePublished["__typename"])
	require.Equal(t, "NpcAvatarNotDraftError", deletePublished["__typename"])
}

func TestDeleteNpcAvatar_RemovesDraftNpcAvatarAndFreesName(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	name := "森の番人-" + npcAvatarUniqueID(t)
	npcAvatar := registerNpcAvatar(t, token, name)
	waitNpcAvatar(t, token, npcAvatar["id"], 1)

	deleted := npcAvatarOperation(t, token, deleteNpcAvatarMutation, "deleteNpcAvatar", npcAvatarInput(npcAvatar, nil))

	require.Equal(t, "DeleteNpcAvatarSuccess", deleted["__typename"])
	require.Equal(t, npcAvatar["id"], deleted["id"])
	registerNpcAvatar(t, token, name)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		found, err := client.TryData(t.Context(), token, npcAvatarQuery, map[string]any{"id": npcAvatar["id"]}, "npcAvatar")
		if assert.NoError(c, err) {
			assert.Equal(c, "NpcAvatarNotFoundError", found["__typename"])
		}
	})
}

func TestNpcAvatarOperations_ReturnResultTypeWhenNotFoundOrNotOperator(t *testing.T) {
	t.Parallel()
	notOperator := key.Sign(t, idptest.ValidClaims("not-operator"))
	unknown := map[string]any{"id": uuid.NewString(), "revision": 1}
	tests := []struct {
		name      string
		token     string
		operation string
		field     string
		variables map[string]any
		want      string
	}{
		{name: "detail_of_unknown", token: operators.Token(t), operation: npcAvatarQuery, field: "npcAvatar",
			variables: map[string]any{"id": uuid.NewString()}, want: "NpcAvatarNotFoundError"},
		{name: "rename_unknown", token: operators.Token(t), operation: renameNpcAvatarMutation, field: "renameNpcAvatar",
			variables: map[string]any{"input": npcAvatarInput(unknown, map[string]any{"name": "森の番人"})}, want: "NpcAvatarNotFoundError"},
		{name: "delete_unknown", token: operators.Token(t), operation: deleteNpcAvatarMutation, field: "deleteNpcAvatar",
			variables: map[string]any{"input": unknown}, want: "NpcAvatarNotFoundError"},
		{name: "publish_unknown", token: operators.Token(t), operation: publishNpcAvatarMutation, field: "publishNpcAvatar",
			variables: map[string]any{"input": unknown}, want: "NpcAvatarNotFoundError"},
		{name: "retire_unknown", token: operators.Token(t), operation: retireNpcAvatarMutation, field: "retireNpcAvatar",
			variables: map[string]any{"input": unknown}, want: "NpcAvatarNotFoundError"},
		{name: "list_by_not_operator", token: notOperator, operation: npcAvatarsListQuery, field: "npcAvatars",
			variables: map[string]any{}, want: "ForbiddenError"},
		{name: "detail_by_not_operator", token: notOperator, operation: npcAvatarQuery, field: "npcAvatar",
			variables: map[string]any{"id": uuid.NewString()}, want: "ForbiddenError"},
		{name: "register_by_not_operator", token: notOperator, operation: registerNpcAvatarMutation, field: "registerNpcAvatar",
			variables: map[string]any{"input": map[string]any{"name": "森の番人-" + npcAvatarUniqueID(t)}}, want: "ForbiddenError"},
		{name: "rename_by_not_operator", token: notOperator, operation: renameNpcAvatarMutation, field: "renameNpcAvatar",
			variables: map[string]any{"input": npcAvatarInput(unknown, map[string]any{"name": "森の番人"})}, want: "ForbiddenError"},
		{name: "delete_by_not_operator", token: notOperator, operation: deleteNpcAvatarMutation, field: "deleteNpcAvatar",
			variables: map[string]any{"input": unknown}, want: "ForbiddenError"},
		{name: "publish_by_not_operator", token: notOperator, operation: publishNpcAvatarMutation, field: "publishNpcAvatar",
			variables: map[string]any{"input": unknown}, want: "ForbiddenError"},
		{name: "retire_by_not_operator", token: notOperator, operation: retireNpcAvatarMutation, field: "retireNpcAvatar",
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
