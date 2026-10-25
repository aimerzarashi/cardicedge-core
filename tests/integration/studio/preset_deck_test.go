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

const presetDecksQuery = `query ($filter: PresetDeckFilter, $first: Int, $after: String) {
	presetDecks(filter: $filter, first: $first, after: $after) {
		__typename
		... on PresetDeckConnection {
			edges { cursor node { ` + presetDeckFields + ` } }
			pageInfo { hasNextPage }
		}
	}
}`

func TestRegisterPresetDeck_ListsRegisteredPresetDeck(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)

	registered := registerPresetDeck(t, token, " 初心者-"+id+" ")

	require.EqualValues(t, 1, registered["revision"])
	var connection map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		connection, err = client.TryData(t.Context(), token, presetDecksQuery, map[string]any{"filter": map[string]any{"nameContains": id}}, "presetDecks")
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
	detail := waitPresetDeck(t, token, registered["id"], 1)
	delete(detail, "__typename")
	require.Equal(t, node, detail)
}

// カード名・種類はカードから引き、カードの一覧と同じ並び（種類の順、同じ種類の中は名前の順）で返す
func TestSetPresetDeckCardCounts_ShowsCardsInCardListOrder(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	spellID := registerPublishedSpell(t, token, "ファイアボール-"+id)
	itemID := registerPublishedItem(t, token, "ポーション-"+id)
	dragonID := registerPublishedCreature(t, token, "ドラゴン-"+id)
	goblinID := registerPublishedCreature(t, token, "ゴブリン-"+id)
	deck := registerPresetDeck(t, token, "初心者-"+id)

	result := setPresetDeckCardCounts(t, token, deck,
		cardCount(spellID, 1), cardCount(itemID, 2), cardCount(dragonID, 3), cardCount(goblinID, 4))

	require.Equal(t, map[string]any{"__typename": "SetPresetDeckCardCountsSuccess", "id": deck["id"], "revision": float64(2)}, result)
	detail := waitPresetDeck(t, token, deck["id"], 2)
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
func TestPresetDeckMutations_ReturnRevisionConflictForStaleRevision(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	stale := registerPresetDeck(t, token, "初心者-"+id)
	renamed := client.Data(t, token, renamePresetDeckMutation, map[string]any{"input": map[string]any{
		"id": stale["id"], "revision": stale["revision"], "name": "中級者-" + id,
	}}, "renamePresetDeck")
	require.Equal(t, "RenamePresetDeckSuccess", renamed["__typename"])
	cardID := registerPublishedCreature(t, token, "ドラゴン-"+id)
	tests := []struct {
		name      string
		mutation  string
		field     string
		variables map[string]any
	}{
		{name: "rename", mutation: renamePresetDeckMutation, field: "renamePresetDeck", variables: map[string]any{
			"id": stale["id"], "revision": stale["revision"], "name": "上級者-" + id,
		}},
		{name: "set_card_counts", mutation: setPresetDeckCardCountsMutation, field: "setPresetDeckCardCounts", variables: map[string]any{
			"id": stale["id"], "revision": stale["revision"], "cards": []any{cardCount(cardID, 1)},
		}},
		{name: "delete", mutation: deletePresetDeckMutation, field: "deletePresetDeck", variables: map[string]any{
			"id": stale["id"], "revision": stale["revision"],
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.Data(t, token, tt.mutation, map[string]any{"input": tt.variables}, tt.field)

			require.Equal(t, "RevisionConflictError", result["__typename"])
		})
	}
	require.Equal(t, "中級者-"+id, waitPresetDeck(t, token, stale["id"], 2)["name"])
}

// 文言は返さず、理由ごとの型と、一覧の中の位置を含む項目名を返す。どれか 1 つでも誤りがあれば何も変えない
func TestSetPresetDeckCardCounts_ReturnsFieldErrorPerCard(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	cardID := registerPublishedCreature(t, token, "ドラゴン-"+id)
	deck := registerPresetDeck(t, token, "初心者-"+id)

	result := setPresetDeckCardCounts(t, token, deck,
		cardCount(cardID, 2), cardCount(uuid.NewString(), 1), cardCount(cardID, 5))

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldNotFound", "field": "cards.1.cardId"},
		map[string]any{"__typename": "FieldDuplicated", "field": "cards.2.cardId"},
		map[string]any{"__typename": "FieldOutOfRange", "field": "cards.2.count", "min": float64(0), "max": float64(4)},
	}}, result)
}

// 一覧が長すぎる・変えた後の合計が多すぎるときは、一覧全体（cards）の誤りとして返す
func TestSetPresetDeckCardCounts_ReturnsFieldErrorForWholeCards(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	deck := registerPresetDeck(t, token, "初心者-"+id)
	full := []map[string]any{}
	for i := range 10 {
		cardID := registerPublishedCreature(t, token, fmt.Sprintf("c%d-%s", i, id))
		full = append(full, cardCount(cardID, 4))
	}
	filled := setPresetDeckCardCounts(t, token, deck, full...)
	tooMany := []map[string]any{}
	for range 41 {
		tooMany = append(tooMany, cardCount(uuid.NewString(), 1))
	}
	extraID := registerPublishedItem(t, token, "ポーション-"+id)
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
			result := setPresetDeckCardCounts(t, token, filled, tt.cards...)

			require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{tt.want}}, result)
		})
	}
}

// 削除すると詳細は見つからなくなり、名前はほかのプリセットデッキが使える
func TestDeletePresetDeck_RemovesPresetDeckAndReleasesName(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	deck := registerPresetDeck(t, token, "初心者-"+id)
	waitPresetDeck(t, token, deck["id"], 1)

	result := client.Data(t, token, deletePresetDeckMutation, map[string]any{"input": map[string]any{
		"id": deck["id"], "revision": deck["revision"],
	}}, "deletePresetDeck")

	require.Equal(t, map[string]any{"__typename": "DeletePresetDeckSuccess", "id": deck["id"]}, result)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		found, err := client.TryData(t.Context(), token, presetDeckQuery, map[string]any{"id": deck["id"]}, "presetDeck")
		if assert.NoError(c, err) {
			assert.Equal(c, "PresetDeckNotFoundError", found["__typename"])
		}
	})
	registerPresetDeck(t, token, "初心者-"+id)
}

func TestPresetDeckOperations_ReturnResultTypeWhenNotFoundOrNotOperator(t *testing.T) {
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
		{name: "detail_of_unknown", token: operators.Token(t), operation: presetDeckQuery, field: "presetDeck",
			variables: map[string]any{"id": uuid.NewString()}, want: "PresetDeckNotFoundError"},
		{name: "rename_unknown", token: operators.Token(t), operation: renamePresetDeckMutation, field: "renamePresetDeck",
			variables: map[string]any{"input": map[string]any{"id": unknown["id"], "revision": 1, "name": "初心者"}}, want: "PresetDeckNotFoundError"},
		{name: "delete_unknown", token: operators.Token(t), operation: deletePresetDeckMutation, field: "deletePresetDeck",
			variables: map[string]any{"input": unknown}, want: "PresetDeckNotFoundError"},
		{name: "list_by_not_operator", token: notOperator, operation: presetDecksQuery, field: "presetDecks",
			variables: map[string]any{}, want: "ForbiddenError"},
		{name: "register_by_not_operator", token: notOperator, operation: registerPresetDeckMutation, field: "registerPresetDeck",
			variables: map[string]any{"input": map[string]any{"name": "初心者-" + testhelper.UniqueID(t)}}, want: "ForbiddenError"},
		{name: "delete_by_not_operator", token: notOperator, operation: deletePresetDeckMutation, field: "deletePresetDeck",
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
