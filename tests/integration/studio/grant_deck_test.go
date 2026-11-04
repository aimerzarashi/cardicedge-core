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

const grantDeckFields = `id name status totalCount revision registeredAt updatedAt
	cards { count card { __typename id name } }`

const grantDeckQuery = `query ($id: UUID!) {
	grantDeck(id: $id) {
		__typename
		... on GrantDeck { ` + grantDeckFields + ` }
	}
}`

const grantDecksListQuery = `query ($filter: GrantDeckFilter, $first: Int, $after: String) {
	grantDecks(filter: $filter, first: $first, after: $after) {
		__typename
		... on GrantDeckConnection {
			edges { cursor node { ` + grantDeckFields + ` } }
			pageInfo { hasNextPage }
		}
		... on ValidationErrors { errors { __typename field } }
	}
}`

const errorFields = `... on ValidationErrors { errors { __typename field ... on FieldOutOfRange { min max } ... on FieldTotalTooLarge { max } ... on FieldTotalMismatch { required } } }`

const registerGrantDeckMutation = `mutation ($input: RegisterGrantDeckInput!) {
	registerGrantDeck(input: $input) {
		__typename
		... on RegisterGrantDeckSuccess { id revision }
		` + errorFields + `
	}
}`

const registerGrantDeckFromPresetDeckMutation = `mutation ($input: RegisterGrantDeckFromPresetDeckInput!) {
	registerGrantDeckFromPresetDeck(input: $input) {
		__typename
		... on RegisterGrantDeckFromPresetDeckSuccess { id revision }
		` + errorFields + `
	}
}`

const renameGrantDeckMutation = `mutation ($input: RenameGrantDeckInput!) {
	renameGrantDeck(input: $input) {
		__typename
		... on RenameGrantDeckSuccess { id revision }
		` + errorFields + `
	}
}`

const setGrantDeckCardCountsMutation = `mutation ($input: SetGrantDeckCardCountsInput!) {
	setGrantDeckCardCounts(input: $input) {
		__typename
		... on SetGrantDeckCardCountsSuccess { id revision }
		` + errorFields + `
	}
}`

const deleteGrantDeckMutation = `mutation ($input: DeleteGrantDeckInput!) {
	deleteGrantDeck(input: $input) {
		__typename
		... on DeleteGrantDeckSuccess { id }
	}
}`

const publishGrantDeckMutation = `mutation ($input: PublishGrantDeckInput!) {
	publishGrantDeck(input: $input) {
		__typename
		... on PublishGrantDeckSuccess { id revision }
		` + errorFields + `
	}
}`

const retireGrantDeckMutation = `mutation ($input: RetireGrantDeckInput!) {
	retireGrantDeck(input: $input) {
		__typename
		... on RetireGrantDeckSuccess { id revision }
	}
}`

func grantDeckOperation(t *testing.T, token, mutation, field string, input map[string]any) map[string]any {
	t.Helper()
	return client.Data(t, token, mutation, map[string]any{"input": input}, field)
}

func registerGrantDeck(t *testing.T, token, name string) map[string]any {
	t.Helper()
	result := grantDeckOperation(t, token, registerGrantDeckMutation, "registerGrantDeck", map[string]any{"name": name})
	require.Equal(t, "RegisterGrantDeckSuccess", result["__typename"])
	return result
}

func grantDeckInput(deck map[string]any, extra map[string]any) map[string]any {
	input := map[string]any{"id": deck["id"], "revision": deck["revision"]}
	for k, v := range extra {
		input[k] = v
	}
	return input
}

func setGrantDeckCardCounts(t *testing.T, token string, deck map[string]any, cards ...map[string]any) map[string]any {
	t.Helper()
	return grantDeckOperation(t, token, setGrantDeckCardCountsMutation, "setGrantDeckCardCounts", grantDeckInput(deck, map[string]any{"cards": cards}))
}

func publishGrantDeck(t *testing.T, token string, deck map[string]any) map[string]any {
	t.Helper()
	return grantDeckOperation(t, token, publishGrantDeckMutation, "publishGrantDeck", grantDeckInput(deck, nil))
}

func retireGrantDeck(t *testing.T, token string, deck map[string]any) map[string]any {
	t.Helper()
	return grantDeckOperation(t, token, retireGrantDeckMutation, "retireGrantDeck", grantDeckInput(deck, nil))
}

// 投影されるまで待ち、revision が want になった詳細を返す
func waitGrantDeck(t *testing.T, token string, id any, revision int) map[string]any {
	t.Helper()
	var deck map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		deck, err = client.TryData(t.Context(), token, grantDeckQuery, map[string]any{"id": id}, "grantDeck")
		if assert.NoError(c, err) {
			assert.Equal(c, "GrantDeck", deck["__typename"])
			assert.EqualValues(c, revision, deck["revision"])
		}
	})
	return deck
}

// 合計がちょうど 40 枚の配布デッキ（4 枚ずつ 10 種類のクリーチャー。準備中）を作り、投影されるまで待つ
func fullGrantDeck(t *testing.T, token, name string) map[string]any {
	t.Helper()
	deck := registerGrantDeck(t, token, name)
	cards := []map[string]any{}
	for i := range 10 {
		cards = append(cards, cardCount(registerPublishedCreature(t, token, fmt.Sprintf("c%d-%s", i, name)), 4))
	}
	filled := setGrantDeckCardCounts(t, token, deck, cards...)
	require.Equal(t, "SetGrantDeckCardCountsSuccess", filled["__typename"])
	return waitGrantDeck(t, token, filled["id"], 2)
}

func TestRegisterGrantDeck_ListsDraftGrantDeck(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)

	registered := registerGrantDeck(t, token, " 初心者-"+id+" ")

	require.EqualValues(t, 1, registered["revision"])
	var connection map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		connection, err = client.TryData(t.Context(), token, grantDecksListQuery, map[string]any{"filter": map[string]any{"nameContains": id}}, "grantDecks")
		if assert.NoError(c, err) {
			assert.Len(c, connection["edges"], 1)
		}
	})
	node := connection["edges"].([]any)[0].(map[string]any)["node"].(map[string]any)
	require.Equal(t, registered["id"], node["id"])
	require.Equal(t, "初心者-"+id, node["name"])
	require.Equal(t, "DRAFT", node["status"])
	require.EqualValues(t, 0, node["totalCount"])
	require.Empty(t, node["cards"])
	require.Equal(t, node["registeredAt"], node["updatedAt"])
	detail := waitGrantDeck(t, token, registered["id"], 1)
	delete(detail, "__typename")
	require.Equal(t, node, detail)
}

func TestGrantDecks_FiltersByStatusAndRejectsEmptyStatuses(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	registered := registerGrantDeck(t, token, "初心者-"+id)
	waitGrantDeck(t, token, registered["id"], 1)

	draft := client.Data(t, token, grantDecksListQuery, map[string]any{"filter": map[string]any{"nameContains": id, "statuses": []any{"DRAFT"}}}, "grantDecks")
	published := client.Data(t, token, grantDecksListQuery, map[string]any{"filter": map[string]any{"nameContains": id, "statuses": []any{"PUBLISHED", "RETIRED"}}}, "grantDecks")
	empty := client.Data(t, token, grantDecksListQuery, map[string]any{"filter": map[string]any{"statuses": []any{}}}, "grantDecks")

	require.Len(t, draft["edges"], 1)
	require.Empty(t, published["edges"])
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldRequired", "field": "filter.statuses"},
	}}, empty)
}

// 写すのはカードと枚数で、名前は指定したもの。写した元は覚えない（作った後は元と関係を持たない）
func TestRegisterGrantDeckFromPresetDeck_CopiesPresetDeck(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	dragonID := registerPublishedCreature(t, token, "ドラゴン-"+id)
	potionID := registerPublishedItem(t, token, "ポーション-"+id)
	preset := registerPresetDeck(t, token, "火-"+id)
	filled := setPresetDeckCardCounts(t, token, preset, cardCount(dragonID, 3), cardCount(potionID, 2))
	presetDetail := waitPresetDeck(t, token, filled["id"], 2)

	copied := grantDeckOperation(t, token, registerGrantDeckFromPresetDeckMutation, "registerGrantDeckFromPresetDeck", map[string]any{"presetDeckId": preset["id"], "name": "炎-" + id})

	require.Equal(t, "RegisterGrantDeckFromPresetDeckSuccess", copied["__typename"])
	require.EqualValues(t, 1, copied["revision"])
	detail := waitGrantDeck(t, token, copied["id"], 1)
	require.Equal(t, "炎-"+id, detail["name"])
	require.Equal(t, "DRAFT", detail["status"])
	require.Equal(t, presetDetail["cards"], detail["cards"])
	require.EqualValues(t, 5, detail["totalCount"])
}

// 名前は元から写さないので、同じプリセットデッキから名前を変えれば何度でも作れる。使われている名前・空の名前は誤りになる
func TestRegisterGrantDeckFromPresetDeck_ReturnsErrorForUnknownPresetDeckOrInvalidName(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	preset := registerPresetDeck(t, token, "火-"+id)
	copyAs := func(name string) map[string]any {
		return grantDeckOperation(t, token, registerGrantDeckFromPresetDeckMutation, "registerGrantDeckFromPresetDeck", map[string]any{"presetDeckId": preset["id"], "name": name})
	}
	first := copyAs("火-" + id)
	require.Equal(t, "RegisterGrantDeckFromPresetDeckSuccess", first["__typename"])

	second := copyAs("火2-" + id)
	again := copyAs("火-" + id)
	blank := copyAs(" ")
	unknown := grantDeckOperation(t, token, registerGrantDeckFromPresetDeckMutation, "registerGrantDeckFromPresetDeck", map[string]any{"presetDeckId": uuid.NewString(), "name": "炎-" + id})

	require.Equal(t, "RegisterGrantDeckFromPresetDeckSuccess", second["__typename"])
	require.Equal(t, "GrantDeckNameAlreadyInUseError", again["__typename"])
	require.Equal(t, "ValidationErrors", blank["__typename"])
	require.Equal(t, []any{map[string]any{"__typename": "FieldRequired", "field": "name"}}, blank["errors"])
	require.Equal(t, "PresetDeckNotFoundError", unknown["__typename"])
}

// 名前は状態によらず、すべての配布デッキの中で一意。削除すると使えるようになる
func TestGrantDeckName_IsUniqueAmongAllGrantDecksRegardlessOfStatus(t *testing.T) {
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	published := fullGrantDeck(t, token, "公開-"+id)
	publishedResult := publishGrantDeck(t, token, published)
	require.Equal(t, "PublishGrantDeckSuccess", publishedResult["__typename"])
	draft := registerGrantDeck(t, token, "準備中-"+id)

	sameAsPublished := grantDeckOperation(t, token, registerGrantDeckMutation, "registerGrantDeck", map[string]any{"name": " 公開-" + id})
	renameToPublished := grantDeckOperation(t, token, renameGrantDeckMutation, "renameGrantDeck", grantDeckInput(draft, map[string]any{"name": "公開-" + id}))
	retired := retireGrantDeck(t, token, ref(publishedResult))
	sameAsRetired := grantDeckOperation(t, token, registerGrantDeckMutation, "registerGrantDeck", map[string]any{"name": "公開-" + id})
	deleted := grantDeckOperation(t, token, deleteGrantDeckMutation, "deleteGrantDeck", grantDeckInput(draft, nil))
	reused := registerGrantDeck(t, token, "準備中-"+id)

	require.Equal(t, "GrantDeckNameAlreadyInUseError", sameAsPublished["__typename"])
	require.Equal(t, "GrantDeckNameAlreadyInUseError", renameToPublished["__typename"])
	require.Equal(t, "RetireGrantDeckSuccess", retired["__typename"])
	require.Equal(t, "GrantDeckNameAlreadyInUseError", sameAsRetired["__typename"])
	require.Equal(t, "DeleteGrantDeckSuccess", deleted["__typename"])
	require.Equal(t, "RegisterGrantDeckSuccess", reused["__typename"])
}

func TestSetGrantDeckCardCounts_ShowsCardsInCardListOrder(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	spellID := registerPublishedSpell(t, token, "ファイアボール-"+id)
	dragonID := registerPublishedCreature(t, token, "ドラゴン-"+id)
	deck := registerGrantDeck(t, token, "初心者-"+id)

	result := setGrantDeckCardCounts(t, token, deck, cardCount(spellID, 1), cardCount(dragonID, 3))

	require.Equal(t, map[string]any{"__typename": "SetGrantDeckCardCountsSuccess", "id": deck["id"], "revision": float64(2)}, result)
	detail := waitGrantDeck(t, token, deck["id"], 2)
	require.Equal(t, []any{
		map[string]any{"count": float64(3), "card": map[string]any{"__typename": "Creature", "id": dragonID, "name": "ドラゴン-" + id}},
		map[string]any{"count": float64(1), "card": map[string]any{"__typename": "Spell", "id": spellID, "name": "ファイアボール-" + id}},
	}, detail["cards"])
}

// 誤りは位置を含む項目名で返し、どれか 1 つでも誤りがあれば何も変えない
func TestSetGrantDeckCardCounts_ReturnsFieldErrorPerCard(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	published := registerPublishedCreature(t, token, "ドラゴン-"+id)
	deck := registerGrantDeck(t, token, "初心者-"+id)

	result := setGrantDeckCardCounts(t, token, deck,
		cardCount(published, 2), cardCount(uuid.NewString(), 1), cardCount(published, 5))

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldNotFound", "field": "cards.1.cardId"},
		map[string]any{"__typename": "FieldDuplicated", "field": "cards.2.cardId"},
		map[string]any{"__typename": "FieldOutOfRange", "field": "cards.2.count", "min": float64(0), "max": float64(4)},
	}}, result)
}

// 新しく入れるカードは公開中だけ（準備中のカードは入れられない）
func TestSetGrantDeckCardCounts_ReturnsUnavailableForCardNotPublished(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	draftCard := registerCreature(t, token, "下書き-"+id)["creature"].(map[string]any)["id"]
	deck := registerGrantDeck(t, token, "初心者-"+id)

	result := setGrantDeckCardCounts(t, token, deck, cardCount(draftCard, 1))

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldUnavailable", "field": "cards.0.cardId"},
	}}, result)
}

// 公開すると、準備中だけの操作（名前・枚数の変更と削除）はできなくなる。使用不可でも同じ
func TestGrantDeckEditing_IsRejectedUnlessDraft(t *testing.T) {
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	deck := fullGrantDeck(t, token, "初心者-"+id)
	published := publishGrantDeck(t, token, deck)
	require.Equal(t, "PublishGrantDeckSuccess", published["__typename"])
	cardID := registerPublishedCreature(t, token, "ドラゴン-"+id)
	states := []struct {
		name string
		deck func() map[string]any
	}{
		{name: "published", deck: func() map[string]any { return ref(published) }},
		{name: "retired", deck: func() map[string]any { return ref(retireGrantDeck(t, token, ref(published))) }},
	}
	for _, s := range states {
		t.Run(s.name, func(t *testing.T) {
			target := s.deck()
			tests := []struct {
				name     string
				mutation string
				field    string
				extra    map[string]any
			}{
				{name: "rename", mutation: renameGrantDeckMutation, field: "renameGrantDeck", extra: map[string]any{"name": "別名-" + id}},
				{name: "set_card_counts", mutation: setGrantDeckCardCountsMutation, field: "setGrantDeckCardCounts", extra: map[string]any{"cards": []any{cardCount(cardID, 1)}}},
				{name: "delete", mutation: deleteGrantDeckMutation, field: "deleteGrantDeck"},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					result := grantDeckOperation(t, token, tt.mutation, tt.field, grantDeckInput(target, tt.extra))

					require.Equal(t, "GrantDeckNotDraftError", result["__typename"])
				})
			}
		})
	}
}

// 公開するとき確かめるのは、合計がちょうど 40 枚・入っているカードがすべて公開中。どちらも ValidationErrors の項目 cards
func TestPublishGrantDeck_ReturnsFieldErrorsWhenTotalOrCardsAreInvalid(t *testing.T) {
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	short := registerGrantDeck(t, token, "少ない-"+id)
	shortFilled := setGrantDeckCardCounts(t, token, short, cardCount(registerPublishedCreature(t, token, "ドラゴン-"+id), 3))
	full := fullGrantDeck(t, token, "満杯-"+id)
	// 入っているカードを使用不可にしても残せる決まりなので、公開するときに確かめ直す
	card := client.Data(t, token, `query { cards(filter: {nameContains: "c0-満杯-`+id+`"}) { ... on CardConnection { edges { node { id revision } } } } }`, nil, "cards")
	node := card["edges"].([]any)[0].(map[string]any)["node"].(map[string]any)
	retired := retireCard(t, token, node["id"], node["revision"])
	require.Equal(t, "RetireCardSuccess", retired["__typename"])

	shortResult := publishGrantDeck(t, token, ref(shortFilled))
	fullResult := publishGrantDeck(t, token, full)

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldTotalMismatch", "field": "cards", "required": float64(40)},
	}}, shortResult)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldUnavailable", "field": "cards"},
	}}, fullResult)
}

// 公開中の数に上限は無い。同じ状態への操作は変わらない
func TestPublishGrantDeck_HasNoLimitAndSameStatusOperationsKeepRevision(t *testing.T) {
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	first := fullGrantDeck(t, token, "一-"+id)
	second := fullGrantDeck(t, token, "二-"+id)
	third := fullGrantDeck(t, token, "三-"+id)
	draftRetire := retireGrantDeck(t, token, first)
	require.Equal(t, "InvalidGrantDeckStatusTransitionError", draftRetire["__typename"])

	firstPublished := publishGrantDeck(t, token, first)
	secondPublished := publishGrantDeck(t, token, second)
	thirdPublished := publishGrantDeck(t, token, third)
	samePublished := publishGrantDeck(t, token, ref(firstPublished))
	retired := retireGrantDeck(t, token, ref(firstPublished))
	sameRetired := retireGrantDeck(t, token, ref(retired))
	republished := publishGrantDeck(t, token, ref(retired))

	require.Equal(t, map[string]any{"__typename": "PublishGrantDeckSuccess", "id": first["id"], "revision": float64(3)}, firstPublished)
	require.Equal(t, "PublishGrantDeckSuccess", secondPublished["__typename"])
	require.Equal(t, "PublishGrantDeckSuccess", thirdPublished["__typename"])
	require.Equal(t, firstPublished, samePublished)
	require.Equal(t, map[string]any{"__typename": "RetireGrantDeckSuccess", "id": first["id"], "revision": float64(4)}, retired)
	require.Equal(t, retired, sameRetired)
	require.Equal(t, "PublishGrantDeckSuccess", republished["__typename"])
}

func TestGrantDeckMutations_ReturnRevisionConflictForStaleRevision(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	stale := registerGrantDeck(t, token, "初心者-"+id)
	renamed := grantDeckOperation(t, token, renameGrantDeckMutation, "renameGrantDeck", grantDeckInput(stale, map[string]any{"name": "中級者-" + id}))
	require.Equal(t, "RenameGrantDeckSuccess", renamed["__typename"])
	cardID := registerPublishedCreature(t, token, "ドラゴン-"+id)
	tests := []struct {
		name     string
		mutation string
		field    string
		extra    map[string]any
	}{
		{name: "rename", mutation: renameGrantDeckMutation, field: "renameGrantDeck", extra: map[string]any{"name": "上級者-" + id}},
		{name: "set_card_counts", mutation: setGrantDeckCardCountsMutation, field: "setGrantDeckCardCounts", extra: map[string]any{"cards": []any{cardCount(cardID, 1)}}},
		{name: "publish", mutation: publishGrantDeckMutation, field: "publishGrantDeck"},
		{name: "retire", mutation: retireGrantDeckMutation, field: "retireGrantDeck"},
		{name: "delete", mutation: deleteGrantDeckMutation, field: "deleteGrantDeck"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := grantDeckOperation(t, token, tt.mutation, tt.field, grantDeckInput(stale, tt.extra))

			require.Equal(t, "RevisionConflictError", result["__typename"])
		})
	}
}

func TestDeleteGrantDeck_RemovesDraftGrantDeck(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	deck := registerGrantDeck(t, token, "初心者-"+testhelper.UniqueID(t))
	waitGrantDeck(t, token, deck["id"], 1)

	result := grantDeckOperation(t, token, deleteGrantDeckMutation, "deleteGrantDeck", grantDeckInput(deck, nil))

	require.Equal(t, map[string]any{"__typename": "DeleteGrantDeckSuccess", "id": deck["id"]}, result)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		found, err := client.TryData(t.Context(), token, grantDeckQuery, map[string]any{"id": deck["id"]}, "grantDeck")
		if assert.NoError(c, err) {
			assert.Equal(c, "GrantDeckNotFoundError", found["__typename"])
		}
	})
}

func TestGrantDeckOperations_ReturnResultTypeWhenNotFoundOrNotOperator(t *testing.T) {
	t.Parallel()
	notOperator := key.Sign(t, idptest.ValidClaims("not-operator"))
	unknown := map[string]any{"id": uuid.NewString(), "revision": 1}
	withExtra := func(extra map[string]any) map[string]any {
		return grantDeckInput(unknown, extra)
	}
	tests := []struct {
		name      string
		token     string
		operation string
		field     string
		variables map[string]any
		want      string
	}{
		{name: "detail_of_unknown", token: operators.Token(t), operation: grantDeckQuery, field: "grantDeck",
			variables: map[string]any{"id": uuid.NewString()}, want: "GrantDeckNotFoundError"},
		{name: "rename_unknown", token: operators.Token(t), operation: renameGrantDeckMutation, field: "renameGrantDeck",
			variables: map[string]any{"input": withExtra(map[string]any{"name": "初心者"})}, want: "GrantDeckNotFoundError"},
		{name: "set_card_counts_unknown", token: operators.Token(t), operation: setGrantDeckCardCountsMutation, field: "setGrantDeckCardCounts",
			variables: map[string]any{"input": withExtra(map[string]any{"cards": []any{}})}, want: "GrantDeckNotFoundError"},
		{name: "delete_unknown", token: operators.Token(t), operation: deleteGrantDeckMutation, field: "deleteGrantDeck",
			variables: map[string]any{"input": unknown}, want: "GrantDeckNotFoundError"},
		{name: "publish_unknown", token: operators.Token(t), operation: publishGrantDeckMutation, field: "publishGrantDeck",
			variables: map[string]any{"input": unknown}, want: "GrantDeckNotFoundError"},
		{name: "retire_unknown", token: operators.Token(t), operation: retireGrantDeckMutation, field: "retireGrantDeck",
			variables: map[string]any{"input": unknown}, want: "GrantDeckNotFoundError"},
		{name: "list_by_not_operator", token: notOperator, operation: grantDecksListQuery, field: "grantDecks",
			variables: map[string]any{}, want: "ForbiddenError"},
		{name: "register_by_not_operator", token: notOperator, operation: registerGrantDeckMutation, field: "registerGrantDeck",
			variables: map[string]any{"input": map[string]any{"name": "初心者-" + testhelper.UniqueID(t)}}, want: "ForbiddenError"},
		{name: "register_from_preset_deck_by_not_operator", token: notOperator, operation: registerGrantDeckFromPresetDeckMutation, field: "registerGrantDeckFromPresetDeck",
			variables: map[string]any{"input": map[string]any{"presetDeckId": uuid.NewString(), "name": "初心者-" + testhelper.UniqueID(t)}}, want: "ForbiddenError"},
		{name: "publish_by_not_operator", token: notOperator, operation: publishGrantDeckMutation, field: "publishGrantDeck",
			variables: map[string]any{"input": unknown}, want: "ForbiddenError"},
		{name: "retire_by_not_operator", token: notOperator, operation: retireGrantDeckMutation, field: "retireGrantDeck",
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
