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

const starterDeckFields = `id name status totalCount revision registeredAt updatedAt
	cards { count card { __typename id name } }
	source { presetDeckId revision changedSinceCopied presetDeckDeleted }`

const starterDeckQuery = `query ($id: UUID!) {
	starterDeck(id: $id) {
		__typename
		... on StarterDeck { ` + starterDeckFields + ` }
	}
}`

const starterDecksListQuery = `query ($filter: StarterDeckFilter, $first: Int, $after: String) {
	starterDecks(filter: $filter, first: $first, after: $after) {
		__typename
		... on StarterDeckConnection {
			edges { cursor node { ` + starterDeckFields + ` } }
			pageInfo { hasNextPage }
		}
		... on ValidationErrors { errors { __typename field } }
	}
}`

const errorFields = `... on ValidationErrors { errors { __typename field ... on FieldOutOfRange { min max } ... on FieldTotalTooLarge { max } ... on FieldTotalMismatch { required } } }`

const registerStarterDeckMutation = `mutation ($input: RegisterStarterDeckInput!) {
	registerStarterDeck(input: $input) {
		__typename
		... on RegisterStarterDeckSuccess { id revision }
		` + errorFields + `
	}
}`

const registerStarterDeckFromPresetDeckMutation = `mutation ($input: RegisterStarterDeckFromPresetDeckInput!) {
	registerStarterDeckFromPresetDeck(input: $input) {
		__typename
		... on RegisterStarterDeckFromPresetDeckSuccess { id revision }
	}
}`

const renameStarterDeckMutation = `mutation ($input: RenameStarterDeckInput!) {
	renameStarterDeck(input: $input) {
		__typename
		... on RenameStarterDeckSuccess { id revision }
		` + errorFields + `
	}
}`

const setStarterDeckCardCountsMutation = `mutation ($input: SetStarterDeckCardCountsInput!) {
	setStarterDeckCardCounts(input: $input) {
		__typename
		... on SetStarterDeckCardCountsSuccess { id revision }
		` + errorFields + `
	}
}`

const deleteStarterDeckMutation = `mutation ($input: DeleteStarterDeckInput!) {
	deleteStarterDeck(input: $input) {
		__typename
		... on DeleteStarterDeckSuccess { id }
	}
}`

const publishStarterDeckMutation = `mutation ($input: PublishStarterDeckInput!) {
	publishStarterDeck(input: $input) {
		__typename
		... on PublishStarterDeckSuccess { id revision }
		` + errorFields + `
	}
}`

const retireStarterDeckMutation = `mutation ($input: RetireStarterDeckInput!) {
	retireStarterDeck(input: $input) {
		__typename
		... on RetireStarterDeckSuccess { id revision }
	}
}`

func starterDeckOperation(t *testing.T, token, mutation, field string, input map[string]any) map[string]any {
	t.Helper()
	return client.Data(t, token, mutation, map[string]any{"input": input}, field)
}

func registerStarterDeck(t *testing.T, token, name string) map[string]any {
	t.Helper()
	result := starterDeckOperation(t, token, registerStarterDeckMutation, "registerStarterDeck", map[string]any{"name": name})
	require.Equal(t, "RegisterStarterDeckSuccess", result["__typename"])
	return result
}

func starterDeckInput(deck map[string]any, extra map[string]any) map[string]any {
	input := map[string]any{"id": deck["id"], "revision": deck["revision"]}
	for k, v := range extra {
		input[k] = v
	}
	return input
}

func setStarterDeckCardCounts(t *testing.T, token string, deck map[string]any, cards ...map[string]any) map[string]any {
	t.Helper()
	return starterDeckOperation(t, token, setStarterDeckCardCountsMutation, "setStarterDeckCardCounts", starterDeckInput(deck, map[string]any{"cards": cards}))
}

func publishStarterDeck(t *testing.T, token string, deck map[string]any) map[string]any {
	t.Helper()
	return starterDeckOperation(t, token, publishStarterDeckMutation, "publishStarterDeck", starterDeckInput(deck, nil))
}

func retireStarterDeck(t *testing.T, token string, deck map[string]any) map[string]any {
	t.Helper()
	return starterDeckOperation(t, token, retireStarterDeckMutation, "retireStarterDeck", starterDeckInput(deck, nil))
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

// 次の操作の入力に渡せるよう、{id, revision} に直す
func ref(result map[string]any) map[string]any {
	return map[string]any{"id": result["id"], "revision": result["revision"]}
}

// 合計がちょうど 40 枚のスターターデッキ（4 枚ずつ 10 種類のクリーチャー。準備中）を作り、投影されるまで待つ
func fullStarterDeck(t *testing.T, token, name string) map[string]any {
	t.Helper()
	deck := registerStarterDeck(t, token, name)
	cards := []map[string]any{}
	for i := range 10 {
		cards = append(cards, cardCount(registerPublishedCreature(t, token, fmt.Sprintf("c%d-%s", i, name)), 4))
	}
	filled := setStarterDeckCardCounts(t, token, deck, cards...)
	require.Equal(t, "SetStarterDeckCardCountsSuccess", filled["__typename"])
	return waitStarterDeck(t, token, filled["id"], 2)
}

// 公開中の数の上限（ゲーム全体で 1 つ）を取り合うため、公開するテストは並列にしない（ほかのテストと取り合うと、
// 上限に達して公開できなくなる。並列のテストは、並列でないテストがすべて終わってから動く）。終わったら使用不可に戻す
func retireOnCleanup(t *testing.T, token string, deck map[string]any) {
	t.Helper()
	t.Cleanup(func() {
		latest, err := client.TryData(t.Context(), token, starterDeckQuery, map[string]any{"id": deck["id"]}, "starterDeck")
		if err == nil && latest["status"] == "PUBLISHED" {
			_, _ = client.TryData(t.Context(), token, retireStarterDeckMutation, map[string]any{"input": ref(latest)}, "retireStarterDeck")
		}
	})
}

func TestRegisterStarterDeck_ListsDraftStarterDeckWithoutSource(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)

	registered := registerStarterDeck(t, token, " 初心者-"+id+" ")

	require.EqualValues(t, 1, registered["revision"])
	var connection map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		connection, err = client.TryData(t.Context(), token, starterDecksListQuery, map[string]any{"filter": map[string]any{"nameContains": id}}, "starterDecks")
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
	require.Nil(t, node["source"])
	require.Equal(t, node["registeredAt"], node["updatedAt"])
	detail := waitStarterDeck(t, token, registered["id"], 1)
	delete(detail, "__typename")
	require.Equal(t, node, detail)
}

func TestStarterDecks_FiltersByStatusAndRejectsEmptyStatuses(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	registered := registerStarterDeck(t, token, "初心者-"+id)
	waitStarterDeck(t, token, registered["id"], 1)

	draft := client.Data(t, token, starterDecksListQuery, map[string]any{"filter": map[string]any{"nameContains": id, "statuses": []any{"DRAFT"}}}, "starterDecks")
	published := client.Data(t, token, starterDecksListQuery, map[string]any{"filter": map[string]any{"nameContains": id, "statuses": []any{"PUBLISHED", "RETIRED"}}}, "starterDecks")
	empty := client.Data(t, token, starterDecksListQuery, map[string]any{"filter": map[string]any{"statuses": []any{}}}, "starterDecks")

	require.Len(t, draft["edges"], 1)
	require.Empty(t, published["edges"])
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldRequired", "field": "filter.statuses"},
	}}, empty)
}

// 写すのは名前・カードと枚数。元の公開 ID と、写したときの revision を覚え、元が変わる・消えると、それが分かる
func TestRegisterStarterDeckFromPresetDeck_CopiesPresetDeckAndTracksSource(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	dragonID := registerPublishedCreature(t, token, "ドラゴン-"+id)
	potionID := registerPublishedItem(t, token, "ポーション-"+id)
	preset := registerPresetDeck(t, token, "火-"+id)
	filled := setPresetDeckCardCounts(t, token, preset, cardCount(dragonID, 3), cardCount(potionID, 2))
	presetDetail := waitPresetDeck(t, token, filled["id"], 2)

	copied := starterDeckOperation(t, token, registerStarterDeckFromPresetDeckMutation, "registerStarterDeckFromPresetDeck", map[string]any{"presetDeckId": preset["id"]})

	require.Equal(t, "RegisterStarterDeckFromPresetDeckSuccess", copied["__typename"])
	require.EqualValues(t, 1, copied["revision"])
	detail := waitStarterDeck(t, token, copied["id"], 1)
	require.Equal(t, "火-"+id, detail["name"])
	require.Equal(t, "DRAFT", detail["status"])
	require.Equal(t, presetDetail["cards"], detail["cards"])
	require.EqualValues(t, 5, detail["totalCount"])
	require.Equal(t, map[string]any{"presetDeckId": preset["id"], "revision": float64(2), "changedSinceCopied": false, "presetDeckDeleted": false}, detail["source"])

	renamed := client.Data(t, token, renamePresetDeckMutation, map[string]any{"input": map[string]any{"id": preset["id"], "revision": 2, "name": "水-" + id}}, "renamePresetDeck")
	require.Equal(t, "RenamePresetDeckSuccess", renamed["__typename"])
	testhelper.Eventually(t, func(c *assert.CollectT) {
		detail, err := client.TryData(t.Context(), token, starterDeckQuery, map[string]any{"id": copied["id"]}, "starterDeck")
		if assert.NoError(c, err) {
			assert.Equal(c, true, detail["source"].(map[string]any)["changedSinceCopied"])
		}
	})
	deleted := client.Data(t, token, deletePresetDeckMutation, map[string]any{"input": map[string]any{"id": preset["id"], "revision": 3}}, "deletePresetDeck")
	require.Equal(t, "DeletePresetDeckSuccess", deleted["__typename"])
	testhelper.Eventually(t, func(c *assert.CollectT) {
		detail, err := client.TryData(t.Context(), token, starterDeckQuery, map[string]any{"id": copied["id"]}, "starterDeck")
		if assert.NoError(c, err) {
			source := detail["source"].(map[string]any)
			assert.Equal(c, true, source["presetDeckDeleted"])
			assert.Equal(c, false, source["changedSinceCopied"])
		}
	})
}

func TestRegisterStarterDeckFromPresetDeck_ReturnsErrorForUnknownPresetDeckOrUsedName(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	preset := registerPresetDeck(t, token, "火-"+id)
	input := map[string]any{"presetDeckId": preset["id"]}
	first := starterDeckOperation(t, token, registerStarterDeckFromPresetDeckMutation, "registerStarterDeckFromPresetDeck", input)
	require.Equal(t, "RegisterStarterDeckFromPresetDeckSuccess", first["__typename"])

	again := starterDeckOperation(t, token, registerStarterDeckFromPresetDeckMutation, "registerStarterDeckFromPresetDeck", input)
	unknown := starterDeckOperation(t, token, registerStarterDeckFromPresetDeckMutation, "registerStarterDeckFromPresetDeck", map[string]any{"presetDeckId": uuid.NewString()})

	require.Equal(t, "StarterDeckNameAlreadyInUseError", again["__typename"])
	require.Equal(t, "PresetDeckNotFoundError", unknown["__typename"])
}

// 名前は状態によらず、すべてのスターターデッキの中で一意。削除すると使えるようになる
func TestStarterDeckName_IsUniqueAmongAllStarterDecksRegardlessOfStatus(t *testing.T) {
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	published := fullStarterDeck(t, token, "公開-"+id)
	publishedResult := publishStarterDeck(t, token, published)
	require.Equal(t, "PublishStarterDeckSuccess", publishedResult["__typename"])
	retireOnCleanup(t, token, published)
	draft := registerStarterDeck(t, token, "準備中-"+id)

	sameAsPublished := starterDeckOperation(t, token, registerStarterDeckMutation, "registerStarterDeck", map[string]any{"name": " 公開-" + id})
	renameToPublished := starterDeckOperation(t, token, renameStarterDeckMutation, "renameStarterDeck", starterDeckInput(draft, map[string]any{"name": "公開-" + id}))
	retired := retireStarterDeck(t, token, ref(publishedResult))
	sameAsRetired := starterDeckOperation(t, token, registerStarterDeckMutation, "registerStarterDeck", map[string]any{"name": "公開-" + id})
	deleted := starterDeckOperation(t, token, deleteStarterDeckMutation, "deleteStarterDeck", starterDeckInput(draft, nil))
	reused := registerStarterDeck(t, token, "準備中-"+id)

	require.Equal(t, "StarterDeckNameAlreadyInUseError", sameAsPublished["__typename"])
	require.Equal(t, "StarterDeckNameAlreadyInUseError", renameToPublished["__typename"])
	require.Equal(t, "RetireStarterDeckSuccess", retired["__typename"])
	require.Equal(t, "StarterDeckNameAlreadyInUseError", sameAsRetired["__typename"])
	require.Equal(t, "DeleteStarterDeckSuccess", deleted["__typename"])
	require.Equal(t, "RegisterStarterDeckSuccess", reused["__typename"])
}

func TestSetStarterDeckCardCounts_ShowsCardsInCardListOrder(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	spellID := registerPublishedSpell(t, token, "ファイアボール-"+id)
	dragonID := registerPublishedCreature(t, token, "ドラゴン-"+id)
	deck := registerStarterDeck(t, token, "初心者-"+id)

	result := setStarterDeckCardCounts(t, token, deck, cardCount(spellID, 1), cardCount(dragonID, 3))

	require.Equal(t, map[string]any{"__typename": "SetStarterDeckCardCountsSuccess", "id": deck["id"], "revision": float64(2)}, result)
	detail := waitStarterDeck(t, token, deck["id"], 2)
	require.Equal(t, []any{
		map[string]any{"count": float64(3), "card": map[string]any{"__typename": "Creature", "id": dragonID, "name": "ドラゴン-" + id}},
		map[string]any{"count": float64(1), "card": map[string]any{"__typename": "Spell", "id": spellID, "name": "ファイアボール-" + id}},
	}, detail["cards"])
}

// 誤りは位置を含む項目名で返し、どれか 1 つでも誤りがあれば何も変えない
func TestSetStarterDeckCardCounts_ReturnsFieldErrorPerCard(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	published := registerPublishedCreature(t, token, "ドラゴン-"+id)
	deck := registerStarterDeck(t, token, "初心者-"+id)

	result := setStarterDeckCardCounts(t, token, deck,
		cardCount(published, 2), cardCount(uuid.NewString(), 1), cardCount(published, 5))

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldNotFound", "field": "cards.1.cardId"},
		map[string]any{"__typename": "FieldDuplicated", "field": "cards.2.cardId"},
		map[string]any{"__typename": "FieldOutOfRange", "field": "cards.2.count", "min": float64(0), "max": float64(4)},
	}}, result)
}

// 新しく入れるカードは公開中だけ（準備中のカードは入れられない）
func TestSetStarterDeckCardCounts_ReturnsUnavailableForCardNotPublished(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	draftCard := registerCreature(t, token, "下書き-"+id)["creature"].(map[string]any)["id"]
	deck := registerStarterDeck(t, token, "初心者-"+id)

	result := setStarterDeckCardCounts(t, token, deck, cardCount(draftCard, 1))

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldUnavailable", "field": "cards.0.cardId"},
	}}, result)
}

// 公開すると、準備中だけの操作（名前・枚数の変更と削除）はできなくなる。使用不可でも同じ
func TestStarterDeckEditing_IsRejectedUnlessDraft(t *testing.T) {
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	deck := fullStarterDeck(t, token, "初心者-"+id)
	published := publishStarterDeck(t, token, deck)
	require.Equal(t, "PublishStarterDeckSuccess", published["__typename"])
	retireOnCleanup(t, token, deck)
	cardID := registerPublishedCreature(t, token, "ドラゴン-"+id)
	states := []struct {
		name string
		deck func() map[string]any
	}{
		{name: "published", deck: func() map[string]any { return ref(published) }},
		{name: "retired", deck: func() map[string]any { return ref(retireStarterDeck(t, token, ref(published))) }},
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
				{name: "rename", mutation: renameStarterDeckMutation, field: "renameStarterDeck", extra: map[string]any{"name": "別名-" + id}},
				{name: "set_card_counts", mutation: setStarterDeckCardCountsMutation, field: "setStarterDeckCardCounts", extra: map[string]any{"cards": []any{cardCount(cardID, 1)}}},
				{name: "delete", mutation: deleteStarterDeckMutation, field: "deleteStarterDeck"},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					result := starterDeckOperation(t, token, tt.mutation, tt.field, starterDeckInput(target, tt.extra))

					require.Equal(t, "StarterDeckNotDraftError", result["__typename"])
				})
			}
		})
	}
}

// 公開するとき確かめるのは、合計がちょうど 40 枚・入っているカードがすべて公開中。どちらも ValidationErrors の項目 cards
func TestPublishStarterDeck_ReturnsFieldErrorsWhenTotalOrCardsAreInvalid(t *testing.T) {
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	short := registerStarterDeck(t, token, "少ない-"+id)
	shortFilled := setStarterDeckCardCounts(t, token, short, cardCount(registerPublishedCreature(t, token, "ドラゴン-"+id), 3))
	full := fullStarterDeck(t, token, "満杯-"+id)
	// 入っているカードを使用不可にしても残せる決まりなので、公開するときに確かめ直す
	card := client.Data(t, token, `query { cards(filter: {nameContains: "c0-満杯-`+id+`"}) { ... on CardConnection { edges { node { id revision } } } } }`, nil, "cards")
	node := card["edges"].([]any)[0].(map[string]any)["node"].(map[string]any)
	retired := retireCard(t, token, node["id"], node["revision"])
	require.Equal(t, "RetireCardSuccess", retired["__typename"])

	shortResult := publishStarterDeck(t, token, ref(shortFilled))
	fullResult := publishStarterDeck(t, token, full)

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldTotalMismatch", "field": "cards", "required": float64(40)},
	}}, shortResult)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldUnavailable", "field": "cards"},
	}}, fullResult)
}

// 公開中の数は上限（既定 2）まで。使用不可にすると数が戻り、もう一度公開できる。同じ状態への操作は変わらない
func TestPublishStarterDeck_StopsAtLimitAndReleasesOnRetire(t *testing.T) {
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	first := fullStarterDeck(t, token, "一-"+id)
	second := fullStarterDeck(t, token, "二-"+id)
	third := fullStarterDeck(t, token, "三-"+id)
	for _, deck := range []map[string]any{first, second, third} {
		retireOnCleanup(t, token, deck)
	}
	draftRetire := retireStarterDeck(t, token, first)
	require.Equal(t, "InvalidStarterDeckStatusTransitionError", draftRetire["__typename"])

	firstPublished := publishStarterDeck(t, token, first)
	secondPublished := publishStarterDeck(t, token, second)
	limitReached := publishStarterDeck(t, token, third)
	samePublished := publishStarterDeck(t, token, ref(firstPublished))
	retired := retireStarterDeck(t, token, ref(firstPublished))
	sameRetired := retireStarterDeck(t, token, ref(retired))
	thirdPublished := publishStarterDeck(t, token, third)
	republishLimit := publishStarterDeck(t, token, ref(retired))

	require.Equal(t, map[string]any{"__typename": "PublishStarterDeckSuccess", "id": first["id"], "revision": float64(3)}, firstPublished)
	require.Equal(t, "PublishStarterDeckSuccess", secondPublished["__typename"])
	require.Equal(t, "StarterDeckLimitReachedError", limitReached["__typename"])
	require.Equal(t, firstPublished, samePublished)
	require.Equal(t, map[string]any{"__typename": "RetireStarterDeckSuccess", "id": first["id"], "revision": float64(4)}, retired)
	require.Equal(t, retired, sameRetired)
	require.Equal(t, "PublishStarterDeckSuccess", thirdPublished["__typename"])
	require.Equal(t, "StarterDeckLimitReachedError", republishLimit["__typename"])
	require.Equal(t, "PUBLISHED", waitStarterDeck(t, token, third["id"], 3)["status"])
}

func TestStarterDeckMutations_ReturnRevisionConflictForStaleRevision(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	stale := registerStarterDeck(t, token, "初心者-"+id)
	renamed := starterDeckOperation(t, token, renameStarterDeckMutation, "renameStarterDeck", starterDeckInput(stale, map[string]any{"name": "中級者-" + id}))
	require.Equal(t, "RenameStarterDeckSuccess", renamed["__typename"])
	cardID := registerPublishedCreature(t, token, "ドラゴン-"+id)
	tests := []struct {
		name     string
		mutation string
		field    string
		extra    map[string]any
	}{
		{name: "rename", mutation: renameStarterDeckMutation, field: "renameStarterDeck", extra: map[string]any{"name": "上級者-" + id}},
		{name: "set_card_counts", mutation: setStarterDeckCardCountsMutation, field: "setStarterDeckCardCounts", extra: map[string]any{"cards": []any{cardCount(cardID, 1)}}},
		{name: "publish", mutation: publishStarterDeckMutation, field: "publishStarterDeck"},
		{name: "retire", mutation: retireStarterDeckMutation, field: "retireStarterDeck"},
		{name: "delete", mutation: deleteStarterDeckMutation, field: "deleteStarterDeck"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := starterDeckOperation(t, token, tt.mutation, tt.field, starterDeckInput(stale, tt.extra))

			require.Equal(t, "RevisionConflictError", result["__typename"])
		})
	}
}

func TestDeleteStarterDeck_RemovesDraftStarterDeck(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	deck := registerStarterDeck(t, token, "初心者-"+testhelper.UniqueID(t))
	waitStarterDeck(t, token, deck["id"], 1)

	result := starterDeckOperation(t, token, deleteStarterDeckMutation, "deleteStarterDeck", starterDeckInput(deck, nil))

	require.Equal(t, map[string]any{"__typename": "DeleteStarterDeckSuccess", "id": deck["id"]}, result)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		found, err := client.TryData(t.Context(), token, starterDeckQuery, map[string]any{"id": deck["id"]}, "starterDeck")
		if assert.NoError(c, err) {
			assert.Equal(c, "StarterDeckNotFoundError", found["__typename"])
		}
	})
}

func TestStarterDeckOperations_ReturnResultTypeWhenNotFoundOrNotOperator(t *testing.T) {
	t.Parallel()
	notOperator := key.Sign(t, idptest.ValidClaims("not-operator"))
	unknown := map[string]any{"id": uuid.NewString(), "revision": 1}
	withExtra := func(extra map[string]any) map[string]any {
		return starterDeckInput(unknown, extra)
	}
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
			variables: map[string]any{"input": withExtra(map[string]any{"name": "初心者"})}, want: "StarterDeckNotFoundError"},
		{name: "set_card_counts_unknown", token: operators.Token(t), operation: setStarterDeckCardCountsMutation, field: "setStarterDeckCardCounts",
			variables: map[string]any{"input": withExtra(map[string]any{"cards": []any{}})}, want: "StarterDeckNotFoundError"},
		{name: "delete_unknown", token: operators.Token(t), operation: deleteStarterDeckMutation, field: "deleteStarterDeck",
			variables: map[string]any{"input": unknown}, want: "StarterDeckNotFoundError"},
		{name: "publish_unknown", token: operators.Token(t), operation: publishStarterDeckMutation, field: "publishStarterDeck",
			variables: map[string]any{"input": unknown}, want: "StarterDeckNotFoundError"},
		{name: "retire_unknown", token: operators.Token(t), operation: retireStarterDeckMutation, field: "retireStarterDeck",
			variables: map[string]any{"input": unknown}, want: "StarterDeckNotFoundError"},
		{name: "list_by_not_operator", token: notOperator, operation: starterDecksListQuery, field: "starterDecks",
			variables: map[string]any{}, want: "ForbiddenError"},
		{name: "register_by_not_operator", token: notOperator, operation: registerStarterDeckMutation, field: "registerStarterDeck",
			variables: map[string]any{"input": map[string]any{"name": "初心者-" + testhelper.UniqueID(t)}}, want: "ForbiddenError"},
		{name: "register_from_preset_deck_by_not_operator", token: notOperator, operation: registerStarterDeckFromPresetDeckMutation, field: "registerStarterDeckFromPresetDeck",
			variables: map[string]any{"input": map[string]any{"presetDeckId": uuid.NewString()}}, want: "ForbiddenError"},
		{name: "publish_by_not_operator", token: notOperator, operation: publishStarterDeckMutation, field: "publishStarterDeck",
			variables: map[string]any{"input": unknown}, want: "ForbiddenError"},
		{name: "retire_by_not_operator", token: notOperator, operation: retireStarterDeckMutation, field: "retireStarterDeck",
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
