package studio_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

// 登録した直後のカードの id と revision。種類によらず同じ操作で状態を変えられることを、3 種類で確かめる
func registerEachKind(t *testing.T, token, id string) map[string]map[string]any {
	t.Helper()
	return map[string]map[string]any{
		"creature": registerCreature(t, token, "ドラゴン-"+id)["creature"].(map[string]any),
		"item":     registerItem(t, token, "ポーション-"+id)["item"].(map[string]any),
		"spell":    registerSpell(t, token, "ファイアボール-"+id)["spell"].(map[string]any),
	}
}

// 投影されるまで待ち、名前に id を含むカードの status と revision を、名前で引ける形で返す
func waitCardStatuses(t *testing.T, token, id, status string, revision float64, want int) map[string]any {
	t.Helper()
	var connection map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		connection, err = client.TryData(t.Context(), token, cardsQuery, map[string]any{"filter": map[string]any{"nameContains": id}}, "cards")
		if !assert.NoError(c, err) {
			return
		}
		edges := connection["edges"].([]any)
		assert.Len(c, edges, want)
		for _, edge := range edges {
			node := edge.(map[string]any)["node"].(map[string]any)
			assert.Equal(c, status, node["status"])
			assert.Equal(c, revision, node["revision"])
		}
	})
	return connection
}

func TestPublishCard_PublishesAndRetiresCardOfAnyKind(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	registered := registerEachKind(t, token, id)

	for _, card := range registered {
		published := publishCard(t, token, card["id"], card["revision"])
		require.Equal(t, map[string]any{"__typename": "PublishCardSuccess", "id": card["id"], "revision": float64(2)}, published)
	}
	waitCardStatuses(t, token, id, "PUBLISHED", 2, 3)
	for _, card := range registered {
		retired := retireCard(t, token, card["id"], float64(2))
		require.Equal(t, map[string]any{"__typename": "RetireCardSuccess", "id": card["id"], "revision": float64(3)}, retired)
	}
	waitCardStatuses(t, token, id, "RETIRED", 3, 3)
	for _, card := range registered {
		republished := publishCard(t, token, card["id"], float64(3))
		require.Equal(t, map[string]any{"__typename": "PublishCardSuccess", "id": card["id"], "revision": float64(4)}, republished)
	}
	waitCardStatuses(t, token, id, "PUBLISHED", 4, 3)
}

// 変わらない操作は成功し、revision を進めない
func TestPublishCard_KeepsRevisionWhenStatusIsUnchanged(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	card := registerCreature(t, token, "ドラゴン-"+testhelper.UniqueID(t))["creature"].(map[string]any)
	published := publishCard(t, token, card["id"], card["revision"])

	publishedAgain := publishCard(t, token, card["id"], published["revision"])
	retired := retireCard(t, token, card["id"], published["revision"])
	retiredAgain := retireCard(t, token, card["id"], retired["revision"])

	require.Equal(t, published, publishedAgain)
	require.Equal(t, retired, retiredAgain)
}

func TestPublishCard_ReturnsResultTypePerReason(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	card := registerCreature(t, token, "ドラゴン-"+id)["creature"].(map[string]any)
	deck := registerPresetDeck(t, token, "初心者-"+id)
	stale := registerItem(t, token, "ポーション-"+id)["item"].(map[string]any)
	publishCard(t, token, stale["id"], stale["revision"])
	tests := []struct {
		name     string
		mutation string
		field    string
		token    string
		input    map[string]any
		want     string
	}{
		{name: "publish_unknown_card", mutation: publishCardMutation, field: "publishCard", token: token, input: map[string]any{"id": uuid.NewString(), "revision": 1}, want: "CardNotFoundError"},
		{name: "publish_not_a_card", mutation: publishCardMutation, field: "publishCard", token: token, input: map[string]any{"id": deck["id"], "revision": 1}, want: "CardNotFoundError"},
		{name: "publish_stale_revision", mutation: publishCardMutation, field: "publishCard", token: token, input: map[string]any{"id": stale["id"], "revision": stale["revision"]}, want: "RevisionConflictError"},
		{name: "publish_not_operator", mutation: publishCardMutation, field: "publishCard", token: key.Sign(t, idptest.ValidClaims("not-operator")), input: map[string]any{"id": card["id"], "revision": card["revision"]}, want: "ForbiddenError"},
		{name: "retire_unknown_card", mutation: retireCardMutation, field: "retireCard", token: token, input: map[string]any{"id": uuid.NewString(), "revision": 1}, want: "CardNotFoundError"},
		{name: "retire_not_a_card", mutation: retireCardMutation, field: "retireCard", token: token, input: map[string]any{"id": deck["id"], "revision": 1}, want: "CardNotFoundError"},
		{name: "retire_stale_revision", mutation: retireCardMutation, field: "retireCard", token: token, input: map[string]any{"id": stale["id"], "revision": stale["revision"]}, want: "RevisionConflictError"},
		{name: "retire_draft", mutation: retireCardMutation, field: "retireCard", token: token, input: map[string]any{"id": card["id"], "revision": card["revision"]}, want: "InvalidCardStatusTransitionError"},
		{name: "retire_not_operator", mutation: retireCardMutation, field: "retireCard", token: key.Sign(t, idptest.ValidClaims("not-operator")), input: map[string]any{"id": card["id"], "revision": card["revision"]}, want: "ForbiddenError"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := client.Post(t, tt.token, tt.mutation, map[string]any{"input": tt.input})

			require.Equal(t, http.StatusOK, status)
			require.Nil(t, body["errors"])
			require.Equal(t, tt.want, body["data"].(map[string]any)[tt.field].(map[string]any)["__typename"])
		})
	}
}

// 指定が無ければすべての状態、指定すればその状態だけ
func TestCards_FiltersByStatusesOnlyWhenGiven(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	registerCreature(t, token, "下書き-"+id)
	registerPublishedCreature(t, token, "公開-"+id)
	retired := registerCreature(t, token, "終了-"+id)["creature"].(map[string]any)
	published := publishCard(t, token, retired["id"], retired["revision"])
	retireCard(t, token, retired["id"], published["revision"])
	testhelper.Eventually(t, func(c *assert.CollectT) {
		connection, err := client.TryData(t.Context(), token, cardsQuery, map[string]any{"filter": map[string]any{"nameContains": id, "statuses": []string{"RETIRED"}}}, "cards")
		if assert.NoError(c, err) {
			assert.Equal(c, []string{"終了-" + id}, cardNames(connection))
		}
	})
	waitCards(t, token, id, 3)
	tests := []struct {
		name     string
		statuses []string
		want     []string
	}{
		{name: "draft", statuses: []string{"DRAFT"}, want: []string{"下書き-" + id}},
		{name: "published", statuses: []string{"PUBLISHED"}, want: []string{"公開-" + id}},
		{name: "draft_and_retired", statuses: []string{"DRAFT", "RETIRED"}, want: []string{"下書き-" + id, "終了-" + id}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connection := listCards(t, token, map[string]any{"filter": map[string]any{"nameContains": id, "statuses": tt.statuses}})

			require.Equal(t, tt.want, cardNames(connection))
		})
	}
	require.Len(t, cardNames(listCards(t, token, map[string]any{"filter": map[string]any{"nameContains": id}})), 3)
}

// 準備中・使用不可のカードを新しく入れると FieldUnavailable。すでに入っているカードは使用不可になっても残せる
func TestSetPresetDeckCardCounts_ReturnsFieldUnavailableForCardsNotPublished(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	deck := registerPresetDeck(t, token, "初心者-"+id)
	draft := registerCreature(t, token, "下書き-"+id)["creature"].(map[string]any)
	retired := registerItem(t, token, "終了-"+id)["item"].(map[string]any)
	published := publishCard(t, token, retired["id"], retired["revision"])
	retireCard(t, token, retired["id"], published["revision"])
	available := registerPublishedSpell(t, token, "公開-"+id)

	result := setPresetDeckCardCounts(t, token, deck, cardCount(draft["id"], 1), cardCount(available, 1), cardCount(retired["id"], 1))

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldUnavailable", "field": "cards.0.cardId"},
		map[string]any{"__typename": "FieldUnavailable", "field": "cards.2.cardId"},
	}}, result)

	added := setPresetDeckCardCounts(t, token, deck, cardCount(available, 2))
	require.Equal(t, "SetPresetDeckCardCountsSuccess", added["__typename"])
	current := waitPresetDeck(t, token, deck["id"], 2)
	retiredSpell := retireCard(t, token, available, float64(2))
	require.Equal(t, "RetireCardSuccess", retiredSpell["__typename"])
	kept := setPresetDeckCardCounts(t, token, current, cardCount(available, 1))
	require.Equal(t, "SetPresetDeckCardCountsSuccess", kept["__typename"])
}
