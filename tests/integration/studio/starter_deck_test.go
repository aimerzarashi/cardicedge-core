package studio_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

const starterDeckFields = `id name totalCount revision registeredAt updatedAt cards { count card { __typename id name } }`

const registerStarterDeckMutation = `mutation ($input: RegisterStarterDeckInput!) {
	registerStarterDeck(input: $input) {
		__typename
		... on RegisterStarterDeckSuccess { id revision }
	}
}`

const renameStarterDeckMutation = `mutation ($input: RenameStarterDeckInput!) {
	renameStarterDeck(input: $input) {
		__typename
		... on RenameStarterDeckSuccess { id revision }
	}
}`

const setStarterDeckCardCountsMutation = `mutation ($input: SetStarterDeckCardCountsInput!) {
	setStarterDeckCardCounts(input: $input) {
		__typename
		... on SetStarterDeckCardCountsSuccess { id revision }
		... on ValidationErrors { errors { __typename field ... on FieldOutOfRange { min max } ... on FieldTotalTooLarge { max } } }
	}
}`

const deleteStarterDeckMutation = `mutation ($input: DeleteStarterDeckInput!) {
	deleteStarterDeck(input: $input) {
		__typename
		... on DeleteStarterDeckSuccess { id }
	}
}`

const starterDeckQuery = `query ($id: UUID!) {
	starterDeck(id: $id) {
		__typename
		... on StarterDeck { ` + starterDeckFields + ` }
	}
}`

const starterDecksQuery = `query ($filter: StarterDeckFilter, $first: Int, $after: String) {
	starterDecks(filter: $filter, first: $first, after: $after) {
		__typename
		... on StarterDeckConnection {
			edges { cursor node { ` + starterDeckFields + ` } }
			pageInfo { hasNextPage }
		}
	}
}`

func registerStarterDeck(t *testing.T, token, name string) map[string]any {
	t.Helper()
	result := client.Data(t, token, registerStarterDeckMutation, map[string]any{"input": map[string]any{"name": name}}, "registerStarterDeck")
	require.Equal(t, "RegisterStarterDeckSuccess", result["__typename"])
	return result
}

func setStarterDeckCardCounts(t *testing.T, token string, deck map[string]any, cards ...map[string]any) map[string]any {
	t.Helper()
	return client.Data(t, token, setStarterDeckCardCountsMutation, map[string]any{"input": map[string]any{
		"id": deck["id"], "revision": deck["revision"], "cards": cards,
	}}, "setStarterDeckCardCounts")
}

func cardCount(cardID any, count int) map[string]any {
	return map[string]any{"cardId": cardID, "count": count}
}

// 投影されるまで待ち、revision が want になった詳細を返す
func waitStarterDeck(t *testing.T, token string, id any, revision int) map[string]any {
	t.Helper()
	var deck map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		deck, err = client.TryData(t.Context(), token, starterDeckQuery, map[string]any{"id": id}, "starterDeck")
		if assert.NoError(c, err) {
			assert.Equal(c, "StarterDeck", deck["__typename"])
			assert.EqualValues(c, revision, deck["revision"])
		}
	})
	return deck
}

func TestRegisterStarterDeck_ListsRegisteredStarterDeck(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)

	registered := registerStarterDeck(t, token, " 初心者-"+id+" ")

	require.EqualValues(t, 1, registered["revision"])
	var connection map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		connection, err = client.TryData(t.Context(), token, starterDecksQuery, map[string]any{"filter": map[string]any{"nameContains": id}}, "starterDecks")
		if assert.NoError(c, err) {
			assert.Len(c, connection["edges"], 1)
		}
	})
	node := connection["edges"].([]any)[0].(map[string]any)["node"].(map[string]any)
	require.Equal(t, registered["id"], node["id"])
	require.Equal(t, "初心者-"+id, node["name"])
	require.EqualValues(t, 1, node["revision"])
	require.EqualValues(t, 0, node["totalCount"])
	require.Empty(t, node["cards"])
	require.Equal(t, node["registeredAt"], node["updatedAt"])
	detail := waitStarterDeck(t, token, registered["id"], 1)
	delete(detail, "__typename")
	require.Equal(t, node, detail)
}

// カード名・種類はカードから引き、カードの一覧と同じ並び（種類の順、同じ種類の中は名前の順）で返す
func TestSetStarterDeckCardCounts_ShowsCardsInCardListOrder(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	spellID := registerSpell(t, token, "ファイアボール-"+id)["spell"].(map[string]any)["id"]
	itemID := registerItem(t, token, "ポーション-"+id)["item"].(map[string]any)["id"]
	dragonID := registerCreature(t, token, "ドラゴン-"+id)["creature"].(map[string]any)["id"]
	goblinID := registerCreature(t, token, "ゴブリン-"+id)["creature"].(map[string]any)["id"]
	deck := registerStarterDeck(t, token, "初心者-"+id)

	result := setStarterDeckCardCounts(t, token, deck,
		cardCount(spellID, 1), cardCount(itemID, 2), cardCount(dragonID, 3), cardCount(goblinID, 4))

	require.Equal(t, map[string]any{"__typename": "SetStarterDeckCardCountsSuccess", "id": deck["id"], "revision": float64(2)}, result)
	detail := waitStarterDeck(t, token, deck["id"], 2)
	want := []any{
		map[string]any{"count": float64(4), "card": map[string]any{"__typename": "Creature", "id": goblinID, "name": "ゴブリン-" + id}},
		map[string]any{"count": float64(3), "card": map[string]any{"__typename": "Creature", "id": dragonID, "name": "ドラゴン-" + id}},
		map[string]any{"count": float64(2), "card": map[string]any{"__typename": "Item", "id": itemID, "name": "ポーション-" + id}},
		map[string]any{"count": float64(1), "card": map[string]any{"__typename": "Spell", "id": spellID, "name": "ファイアボール-" + id}},
	}
	require.Equal(t, want, detail["cards"])
	require.EqualValues(t, 10, detail["totalCount"])
}

// 画面が見ていた revision が古ければ、何も変えずに RevisionConflictError を返す
func TestStarterDeckMutations_ReturnRevisionConflictForStaleRevision(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	stale := registerStarterDeck(t, token, "初心者-"+id)
	renamed := client.Data(t, token, renameStarterDeckMutation, map[string]any{"input": map[string]any{
		"id": stale["id"], "revision": stale["revision"], "name": "中級者-" + id,
	}}, "renameStarterDeck")
	require.Equal(t, "RenameStarterDeckSuccess", renamed["__typename"])
	cardID := registerCreature(t, token, "ドラゴン-"+id)["creature"].(map[string]any)["id"]
	tests := []struct {
		name      string
		mutation  string
		field     string
		variables map[string]any
	}{
		{name: "rename", mutation: renameStarterDeckMutation, field: "renameStarterDeck", variables: map[string]any{
			"id": stale["id"], "revision": stale["revision"], "name": "上級者-" + id,
		}},
		{name: "set_card_counts", mutation: setStarterDeckCardCountsMutation, field: "setStarterDeckCardCounts", variables: map[string]any{
			"id": stale["id"], "revision": stale["revision"], "cards": []any{cardCount(cardID, 1)},
		}},
		{name: "delete", mutation: deleteStarterDeckMutation, field: "deleteStarterDeck", variables: map[string]any{
			"id": stale["id"], "revision": stale["revision"],
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.Data(t, token, tt.mutation, map[string]any{"input": tt.variables}, tt.field)

			require.Equal(t, "RevisionConflictError", result["__typename"])
		})
	}
	require.Equal(t, "中級者-"+id, waitStarterDeck(t, token, stale["id"], 2)["name"])
}

// 文言は返さず、理由ごとの型と、一覧の中の位置を含む項目名を返す。どれか 1 つでも誤りがあれば何も変えない
func TestSetStarterDeckCardCounts_ReturnsFieldErrorPerCard(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	cardID := registerCreature(t, token, "ドラゴン-"+id)["creature"].(map[string]any)["id"]
	deck := registerStarterDeck(t, token, "初心者-"+id)

	result := setStarterDeckCardCounts(t, token, deck,
		cardCount(cardID, 2), cardCount(uuid.NewString(), 1), cardCount(cardID, 5))

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldNotFound", "field": "cards.1.cardId"},
		map[string]any{"__typename": "FieldDuplicated", "field": "cards.2.cardId"},
		map[string]any{"__typename": "FieldOutOfRange", "field": "cards.2.count", "min": float64(0), "max": float64(4)},
	}}, result)
}

// 一覧が長すぎる・変えた後の合計が多すぎるときは、一覧全体（cards）の誤りとして返す
func TestSetStarterDeckCardCounts_ReturnsFieldErrorForWholeCards(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	deck := registerStarterDeck(t, token, "初心者-"+id)
	full := []map[string]any{}
	for i := range 10 {
		cardID := registerCreature(t, token, fmt.Sprintf("c%d-%s", i, id))["creature"].(map[string]any)["id"]
		full = append(full, cardCount(cardID, 4))
	}
	filled := setStarterDeckCardCounts(t, token, deck, full...)
	tooMany := []map[string]any{}
	for range 41 {
		tooMany = append(tooMany, cardCount(uuid.NewString(), 1))
	}
	extraID := registerItem(t, token, "ポーション-"+id)["item"].(map[string]any)["id"]
	tests := []struct {
		name  string
		cards []map[string]any
		want  map[string]any
	}{
		{name: "too_many_cards", cards: tooMany, want: map[string]any{"__typename": "FieldOutOfRange", "field": "cards", "min": float64(0), "max": float64(40)}},
		{name: "total_over_max", cards: []map[string]any{cardCount(extraID, 1)}, want: map[string]any{"__typename": "FieldTotalTooLarge", "field": "cards", "max": float64(40)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := setStarterDeckCardCounts(t, token, filled, tt.cards...)

			require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{tt.want}}, result)
		})
	}
}

// 削除すると詳細は見つからなくなり、名前はほかのスターターデッキが使える
func TestDeleteStarterDeck_RemovesStarterDeckAndReleasesName(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	deck := registerStarterDeck(t, token, "初心者-"+id)
	waitStarterDeck(t, token, deck["id"], 1)

	result := client.Data(t, token, deleteStarterDeckMutation, map[string]any{"input": map[string]any{
		"id": deck["id"], "revision": deck["revision"],
	}}, "deleteStarterDeck")

	require.Equal(t, map[string]any{"__typename": "DeleteStarterDeckSuccess", "id": deck["id"]}, result)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		found, err := client.TryData(t.Context(), token, starterDeckQuery, map[string]any{"id": deck["id"]}, "starterDeck")
		if assert.NoError(c, err) {
			assert.Equal(c, "StarterDeckNotFoundError", found["__typename"])
		}
	})
	registerStarterDeck(t, token, "初心者-"+id)
}

func TestStarterDeckOperations_ReturnResultTypeWhenNotFoundOrNotOperator(t *testing.T) {
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
		{name: "detail_of_unknown", token: operators.Token(t), operation: starterDeckQuery, field: "starterDeck",
			variables: map[string]any{"id": uuid.NewString()}, want: "StarterDeckNotFoundError"},
		{name: "rename_unknown", token: operators.Token(t), operation: renameStarterDeckMutation, field: "renameStarterDeck",
			variables: map[string]any{"input": map[string]any{"id": unknown["id"], "revision": 1, "name": "初心者"}}, want: "StarterDeckNotFoundError"},
		{name: "delete_unknown", token: operators.Token(t), operation: deleteStarterDeckMutation, field: "deleteStarterDeck",
			variables: map[string]any{"input": unknown}, want: "StarterDeckNotFoundError"},
		{name: "list_by_not_operator", token: notOperator, operation: starterDecksQuery, field: "starterDecks",
			variables: map[string]any{}, want: "ForbiddenError"},
		{name: "register_by_not_operator", token: notOperator, operation: registerStarterDeckMutation, field: "registerStarterDeck",
			variables: map[string]any{"input": map[string]any{"name": "初心者-" + testhelper.UniqueID(t)}}, want: "ForbiddenError"},
		{name: "delete_by_not_operator", token: notOperator, operation: deleteStarterDeckMutation, field: "deleteStarterDeck",
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
