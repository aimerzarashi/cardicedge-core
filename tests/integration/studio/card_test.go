package studio_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

const cardsQuery = `query ($filter: CardFilter, $first: Int, $after: String) {
	cards(filter: $filter, first: $first, after: $after) {
		__typename
		... on CardConnection {
			edges { cursor node { __typename id name registeredAt updatedAt } }
			pageInfo { hasNextPage hasPreviousPage startCursor endCursor }
		}
		... on ValidationErrors { errors { __typename field ... on FieldTooLong { max } ... on FieldOutOfRange { min max } } }
	}
}`

func listCards(t *testing.T, token string, variables map[string]any) map[string]any {
	t.Helper()
	return client.Data(t, token, cardsQuery, variables, "cards")
}

// 投影されるまで待ち、名前に id を含むカードが want 件そろった一覧を返す
func waitCards(t *testing.T, token, id string, want int) map[string]any {
	t.Helper()
	var connection map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		connection, err = client.TryData(t.Context(), token, cardsQuery, map[string]any{"filter": map[string]any{"nameContains": id}}, "cards")
		if assert.NoError(c, err) {
			assert.Len(c, connection["edges"], want)
		}
	})
	return connection
}

// edges の node の名前を並べる
func cardNames(connection map[string]any) []string {
	names := []string{}
	for _, edge := range connection["edges"].([]any) {
		names = append(names, edge.(map[string]any)["node"].(map[string]any)["name"].(string))
	}
	return names
}

func TestCards_ListsRegisteredCreature(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	registered := registerCreature(t, token, "ドラゴン-"+id)["creature"].(map[string]any)

	connection := waitCards(t, token, id, 1)

	edge := connection["edges"].([]any)[0].(map[string]any)
	require.Equal(t, "CardConnection", connection["__typename"])
	require.Equal(t, map[string]any{
		"__typename":   "Creature",
		"id":           registered["id"],
		"name":         registered["name"],
		"registeredAt": registered["registeredAt"],
		"updatedAt":    registered["updatedAt"],
	}, edge["node"])
	require.NotEmpty(t, edge["cursor"])
	require.Equal(t, map[string]any{
		"hasNextPage":     false,
		"hasPreviousPage": false,
		"startCursor":     edge["cursor"],
		"endCursor":       edge["cursor"],
	}, connection["pageInfo"])
}

func TestCards_ListsRegisteredItem(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	registered := registerItem(t, token, "ポーション-"+id)["item"].(map[string]any)

	connection := waitCards(t, token, id, 1)

	edge := connection["edges"].([]any)[0].(map[string]any)
	require.Equal(t, map[string]any{
		"__typename":   "Item",
		"id":           registered["id"],
		"name":         registered["name"],
		"registeredAt": registered["registeredAt"],
		"updatedAt":    registered["updatedAt"],
	}, edge["node"])
}

func TestCards_PagesForwardFromAfterCursor(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	registerCreature(t, token, "a-"+id)
	registerCreature(t, token, "b-"+id)
	waitCards(t, token, id, 2)
	filter := map[string]any{"nameContains": id}

	first := listCards(t, token, map[string]any{"filter": filter, "first": 1})
	firstPage := first["pageInfo"].(map[string]any)
	second := listCards(t, token, map[string]any{"filter": filter, "first": 1, "after": firstPage["endCursor"]})
	secondPage := second["pageInfo"].(map[string]any)

	require.Equal(t, []string{"a-" + id}, cardNames(first))
	require.Equal(t, true, firstPage["hasNextPage"])
	require.Equal(t, false, firstPage["hasPreviousPage"])
	require.Equal(t, []string{"b-" + id}, cardNames(second))
	require.Equal(t, false, secondPage["hasNextPage"])
	require.Equal(t, true, secondPage["hasPreviousPage"])
}

// 種類を指定しなければすべての種類、指定すればその種類だけ
func TestCards_FiltersByTypesOnlyWhenGiven(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	registerCreature(t, token, "ドラゴン-"+id)
	waitCards(t, token, id, 1)
	tests := []struct {
		name   string
		filter map[string]any
		want   []string
	}{
		{name: "without_types", filter: map[string]any{"nameContains": id}, want: []string{"ドラゴン-" + id}},
		{name: "creature", filter: map[string]any{"nameContains": id, "types": []string{"CREATURE"}}, want: []string{"ドラゴン-" + id}},
		{name: "other_types", filter: map[string]any{"nameContains": id, "types": []string{"ITEM", "SPELL"}}, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connection := listCards(t, token, map[string]any{"filter": tt.filter})

			require.Equal(t, tt.want, cardNames(connection))
		})
	}
}

func TestCards_ReturnsForbiddenErrorForNonOperator(t *testing.T) {
	t.Parallel()

	result := listCards(t, key.Sign(t, idptest.ValidClaims("not-operator")), nil)

	require.Equal(t, "ForbiddenError", result["__typename"])
}

// 入れ子の入力は、項目の場所（filter.nameContains 等）で返す
func TestCards_ReturnsFieldErrorPerReason(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	tests := []struct {
		name      string
		variables map[string]any
		want      map[string]any
	}{
		{name: "first_below_min", variables: map[string]any{"first": 0}, want: map[string]any{"__typename": "FieldOutOfRange", "field": "first", "min": float64(1), "max": float64(100)}},
		{name: "first_above_max", variables: map[string]any{"first": 101}, want: map[string]any{"__typename": "FieldOutOfRange", "field": "first", "min": float64(1), "max": float64(100)}},
		{name: "malformed_after", variables: map[string]any{"after": "not-a-cursor"}, want: map[string]any{"__typename": "FieldInvalidFormat", "field": "after"}},
		{name: "blank_name_contains", variables: map[string]any{"filter": map[string]any{"nameContains": " "}}, want: map[string]any{"__typename": "FieldRequired", "field": "filter.nameContains"}},
		{name: "too_long_name_contains", variables: map[string]any{"filter": map[string]any{"nameContains": strings.Repeat("あ", 51)}}, want: map[string]any{"__typename": "FieldTooLong", "field": "filter.nameContains", "max": float64(50)}},
		{name: "empty_types", variables: map[string]any{"filter": map[string]any{"types": []string{}}}, want: map[string]any{"__typename": "FieldRequired", "field": "filter.types"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := listCards(t, token, tt.variables)

			require.Equal(t, "ValidationErrors", result["__typename"])
			require.Equal(t, []any{tt.want}, result["errors"])
		})
	}
}
