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

const npcAvatarFields = `id name status revision registeredAt updatedAt decks { id name status }`

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

const setNpcAvatarDecksMutation = `mutation ($input: SetNpcAvatarDecksInput!) {
	setNpcAvatarDecks(input: $input) {
		__typename
		... on SetNpcAvatarDecksSuccess { id revision }
		` + npcAvatarErrorFields + `
	}
}`

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
		` + npcAvatarErrorFields + `
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

// 公開中の NPC のデッキ（40 枚）を作り、投影されるまで待つ
func publishedNpcDeck(t *testing.T, token, name string) map[string]any {
	t.Helper()
	deck := fullNpcDeck(t, token, name)
	published := publishNpcDeck(t, token, deck)
	require.Equal(t, "PublishNpcDeckSuccess", published["__typename"])
	return waitNpcDeck(t, token, published["id"], 3)
}

func setNpcAvatarDecks(t *testing.T, token string, npcAvatar map[string]any, decks ...any) map[string]any {
	t.Helper()
	// deckIds は必須（空でも [] を送る）
	return npcAvatarOperation(t, token, setNpcAvatarDecksMutation, "setNpcAvatarDecks", npcAvatarInput(npcAvatar, map[string]any{"deckIds": append([]any{}, decks...)}))
}

// 公開中の NPC のデッキを 1 つ使う、準備中の NPC のアバター（変更の結果。id と revision を持つ）
func readyNpcAvatar(t *testing.T, token, name string) map[string]any {
	t.Helper()
	npcAvatar := registerNpcAvatar(t, token, name)
	deck := publishedNpcDeck(t, token, "デッキ-"+name)
	assigned := setNpcAvatarDecks(t, token, npcAvatar, deck["id"])
	require.Equal(t, "SetNpcAvatarDecksSuccess", assigned["__typename"])
	return assigned
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
	ready := readyNpcAvatar(t, token, "完成-"+id)
	published := npcAvatarOperation(t, token, publishNpcAvatarMutation, "publishNpcAvatar", npcAvatarInput(ready, nil))
	require.Equal(t, "PublishNpcAvatarSuccess", published["__typename"])
	waitNpcAvatar(t, token, published["id"], 3)
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
	npcAvatar := readyNpcAvatar(t, token, "森の番人-"+npcAvatarUniqueID(t))

	published := npcAvatarOperation(t, token, publishNpcAvatarMutation, "publishNpcAvatar", npcAvatarInput(npcAvatar, nil))
	again := npcAvatarOperation(t, token, publishNpcAvatarMutation, "publishNpcAvatar", npcAvatarInput(published, nil))
	retired := npcAvatarOperation(t, token, retireNpcAvatarMutation, "retireNpcAvatar", npcAvatarInput(published, nil))
	republished := npcAvatarOperation(t, token, publishNpcAvatarMutation, "publishNpcAvatar", npcAvatarInput(retired, nil))

	require.EqualValues(t, 3, published["revision"])
	require.Equal(t, published["revision"], again["revision"])
	require.EqualValues(t, 4, retired["revision"])
	require.EqualValues(t, 5, republished["revision"])
	require.Equal(t, "PUBLISHED", waitNpcAvatar(t, token, npcAvatar["id"], 5)["status"])
}

func TestNpcAvatarOperations_ReturnErrorTypesForNameStatusAndRevision(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := npcAvatarUniqueID(t)
	registerNpcAvatar(t, token, "森の番人-"+id)
	npcAvatar := readyNpcAvatar(t, token, "炎の剣士-"+id)

	used := npcAvatarOperation(t, token, renameNpcAvatarMutation, "renameNpcAvatar", npcAvatarInput(npcAvatar, map[string]any{"name": " 森の番人-" + id}))
	retireDraft := npcAvatarOperation(t, token, retireNpcAvatarMutation, "retireNpcAvatar", npcAvatarInput(npcAvatar, nil))
	stale := npcAvatarOperation(t, token, renameNpcAvatarMutation, "renameNpcAvatar", map[string]any{"id": npcAvatar["id"], "revision": 7, "name": "湖-" + id})
	published := npcAvatarOperation(t, token, publishNpcAvatarMutation, "publishNpcAvatar", npcAvatarInput(npcAvatar, nil))
	renamePublished := npcAvatarOperation(t, token, renameNpcAvatarMutation, "renameNpcAvatar", npcAvatarInput(published, map[string]any{"name": "湖-" + id}))
	deletePublished := npcAvatarOperation(t, token, deleteNpcAvatarMutation, "deleteNpcAvatar", npcAvatarInput(published, nil))
	changeDecksPublished := setNpcAvatarDecks(t, token, published)

	require.Equal(t, "NpcAvatarNameAlreadyInUseError", used["__typename"])
	require.Equal(t, "InvalidNpcAvatarStatusTransitionError", retireDraft["__typename"])
	require.Equal(t, "RevisionConflictError", stale["__typename"])
	require.Equal(t, "NpcAvatarNotDraftError", renamePublished["__typename"])
	require.Equal(t, "NpcAvatarNotDraftError", deletePublished["__typename"])
	require.Equal(t, "NpcAvatarNotDraftError", changeDecksPublished["__typename"])
}

func TestSetNpcAvatarDecks_ShowsDecksInGivenOrderAndReplacesThem(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := npcAvatarUniqueID(t)
	npcAvatar := registerNpcAvatar(t, token, "森の番人-"+id)
	first := publishedNpcDeck(t, token, "一つ目-"+id)
	second := publishedNpcDeck(t, token, "二つ目-"+id)

	changed := setNpcAvatarDecks(t, token, npcAvatar, second["id"], first["id"])
	same := setNpcAvatarDecks(t, token, changed, first["id"], second["id"])
	require.Equal(t, "SetNpcAvatarDecksSuccess", changed["__typename"])
	require.EqualValues(t, 2, changed["revision"])
	require.Equal(t, changed, same)
	detail := waitNpcAvatar(t, token, npcAvatar["id"], 2)
	require.Equal(t, []any{
		map[string]any{"id": second["id"], "name": "二つ目-" + id, "status": "PUBLISHED"},
		map[string]any{"id": first["id"], "name": "一つ目-" + id, "status": "PUBLISHED"},
	}, detail["decks"])

	replaced := setNpcAvatarDecks(t, token, changed, first["id"])

	require.EqualValues(t, 3, replaced["revision"])
	require.Len(t, waitNpcAvatar(t, token, npcAvatar["id"], 3)["decks"], 1)
}

func TestSetNpcAvatarDecks_ReturnsFieldErrorPerDeck(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := npcAvatarUniqueID(t)
	npcAvatar := registerNpcAvatar(t, token, "森の番人-"+id)
	published := publishedNpcDeck(t, token, "公開-"+id)
	draft := fullNpcDeck(t, token, "準備中-"+id)

	duplicated := setNpcAvatarDecks(t, token, npcAvatar, published["id"], published["id"])
	unknown := setNpcAvatarDecks(t, token, npcAvatar, uuid.NewString())
	notPublished := setNpcAvatarDecks(t, token, npcAvatar, draft["id"], published["id"])

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldDuplicated", "field": "deckIds.1"},
	}}, duplicated)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldNotFound", "field": "deckIds.0"},
	}}, unknown)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldUnavailable", "field": "deckIds.0"},
	}}, notPublished)
}

func TestPublishNpcAvatar_ReturnsFieldErrorWhenNoDeckIsPublished(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := npcAvatarUniqueID(t)
	noDeck := registerNpcAvatar(t, token, "デッキ無し-"+id)
	retiredDeck := readyNpcAvatar(t, token, "使用不可-"+id)
	deck := waitNpcAvatar(t, token, retiredDeck["id"], 2)["decks"].([]any)[0].(map[string]any)
	deckDetail := waitNpcDeck(t, token, deck["id"], 3)
	retired := retireNpcDeck(t, token, deckDetail)
	require.Equal(t, "RetireNpcDeckSuccess", retired["__typename"])

	noDeckResult := npcAvatarOperation(t, token, publishNpcAvatarMutation, "publishNpcAvatar", npcAvatarInput(noDeck, nil))
	retiredResult := npcAvatarOperation(t, token, publishNpcAvatarMutation, "publishNpcAvatar", npcAvatarInput(retiredDeck, nil))

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldRequired", "field": "decks"},
	}}, noDeckResult)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldUnavailable", "field": "decks"},
	}}, retiredResult)
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
		{name: "set_decks_unknown", token: operators.Token(t), operation: setNpcAvatarDecksMutation, field: "setNpcAvatarDecks",
			variables: map[string]any{"input": npcAvatarInput(unknown, map[string]any{"deckIds": []string{}})}, want: "NpcAvatarNotFoundError"},
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
		{name: "set_decks_by_not_operator", token: notOperator, operation: setNpcAvatarDecksMutation, field: "setNpcAvatarDecks",
			variables: map[string]any{"input": npcAvatarInput(unknown, map[string]any{"deckIds": []string{}})}, want: "ForbiddenError"},
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
