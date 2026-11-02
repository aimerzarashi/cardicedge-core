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

const npcDeckFields = `id name status totalCount revision registeredAt updatedAt
	cards { count card { __typename id name } }
	source { presetDeckId revision changedSinceCopied presetDeckDeleted }`

const npcDeckQuery = `query ($id: UUID!) {
	npcDeck(id: $id) {
		__typename
		... on NpcDeck { ` + npcDeckFields + ` }
	}
}`

const npcDecksListQuery = `query ($filter: NpcDeckFilter, $first: Int, $after: String) {
	npcDecks(filter: $filter, first: $first, after: $after) {
		__typename
		... on NpcDeckConnection {
			edges { cursor node { ` + npcDeckFields + ` } }
			pageInfo { hasNextPage }
		}
		... on ValidationErrors { errors { __typename field } }
	}
}`

const npcDeckErrorFields = `... on ValidationErrors { errors { __typename field ... on FieldOutOfRange { min max } ... on FieldTotalTooLarge { max } ... on FieldTotalMismatch { required } } }`

const registerNpcDeckMutation = `mutation ($input: RegisterNpcDeckInput!) {
	registerNpcDeck(input: $input) {
		__typename
		... on RegisterNpcDeckSuccess { id revision }
		` + npcDeckErrorFields + `
	}
}`

const registerNpcDeckFromPresetDeckMutation = `mutation ($input: RegisterNpcDeckFromPresetDeckInput!) {
	registerNpcDeckFromPresetDeck(input: $input) {
		__typename
		... on RegisterNpcDeckFromPresetDeckSuccess { id revision }
		` + npcDeckErrorFields + `
	}
}`

const renameNpcDeckMutation = `mutation ($input: RenameNpcDeckInput!) {
	renameNpcDeck(input: $input) {
		__typename
		... on RenameNpcDeckSuccess { id revision }
		` + npcDeckErrorFields + `
	}
}`

const setNpcDeckCardCountsMutation = `mutation ($input: SetNpcDeckCardCountsInput!) {
	setNpcDeckCardCounts(input: $input) {
		__typename
		... on SetNpcDeckCardCountsSuccess { id revision }
		` + npcDeckErrorFields + `
	}
}`

const deleteNpcDeckMutation = `mutation ($input: DeleteNpcDeckInput!) {
	deleteNpcDeck(input: $input) {
		__typename
		... on DeleteNpcDeckSuccess { id }
	}
}`

const publishNpcDeckMutation = `mutation ($input: PublishNpcDeckInput!) {
	publishNpcDeck(input: $input) {
		__typename
		... on PublishNpcDeckSuccess { id revision }
		` + npcDeckErrorFields + `
	}
}`

const retireNpcDeckMutation = `mutation ($input: RetireNpcDeckInput!) {
	retireNpcDeck(input: $input) {
		__typename
		... on RetireNpcDeckSuccess { id revision }
	}
}`

func npcDeckOperation(t *testing.T, token, mutation, field string, input map[string]any) map[string]any {
	t.Helper()
	return client.Data(t, token, mutation, map[string]any{"input": input}, field)
}

func registerNpcDeck(t *testing.T, token, name string) map[string]any {
	t.Helper()
	result := npcDeckOperation(t, token, registerNpcDeckMutation, "registerNpcDeck", map[string]any{"name": name})
	require.Equal(t, "RegisterNpcDeckSuccess", result["__typename"])
	return result
}

func npcDeckInput(npcDeck map[string]any, extra map[string]any) map[string]any {
	input := map[string]any{"id": npcDeck["id"], "revision": npcDeck["revision"]}
	for k, v := range extra {
		input[k] = v
	}
	return input
}

func setNpcDeckCardCounts(t *testing.T, token string, npcDeck map[string]any, cards ...map[string]any) map[string]any {
	t.Helper()
	return npcDeckOperation(t, token, setNpcDeckCardCountsMutation, "setNpcDeckCardCounts", npcDeckInput(npcDeck, map[string]any{"cards": cards}))
}

func publishNpcDeck(t *testing.T, token string, npcDeck map[string]any) map[string]any {
	t.Helper()
	return npcDeckOperation(t, token, publishNpcDeckMutation, "publishNpcDeck", npcDeckInput(npcDeck, nil))
}

func retireNpcDeck(t *testing.T, token string, npcDeck map[string]any) map[string]any {
	t.Helper()
	return npcDeckOperation(t, token, retireNpcDeckMutation, "retireNpcDeck", npcDeckInput(npcDeck, nil))
}

// 投影されるまで待ち、revision が want になった詳細を返す
func waitNpcDeck(t *testing.T, token string, id any, revision int) map[string]any {
	t.Helper()
	var npcDeck map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		npcDeck, err = client.TryData(t.Context(), token, npcDeckQuery, map[string]any{"id": id}, "npcDeck")
		if assert.NoError(c, err) {
			assert.Equal(c, "NpcDeck", npcDeck["__typename"])
			assert.EqualValues(c, revision, npcDeck["revision"])
		}
	})
	return npcDeck
}

// 合計がちょうど 40 枚の NPC のデッキ（4 枚ずつ 10 種類のクリーチャー。準備中）を作り、投影されるまで待つ
func fullNpcDeck(t *testing.T, token, name string) map[string]any {
	t.Helper()
	npcDeck := registerNpcDeck(t, token, name)
	cards := []map[string]any{}
	for i := range 10 {
		cards = append(cards, cardCount(registerPublishedCreature(t, token, fmt.Sprintf("c%d-%s", i, name)), 4))
	}
	filled := setNpcDeckCardCounts(t, token, npcDeck, cards...)
	require.Equal(t, "SetNpcDeckCardCountsSuccess", filled["__typename"])
	return waitNpcDeck(t, token, filled["id"], 2)
}

func TestRegisterNpcDeck_ListsDraftNpcDeckWithoutSource(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)

	registered := registerNpcDeck(t, token, " 初心者-"+id+" ")

	require.EqualValues(t, 1, registered["revision"])
	var connection map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		connection, err = client.TryData(t.Context(), token, npcDecksListQuery, map[string]any{"filter": map[string]any{"nameContains": id}}, "npcDecks")
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
	detail := waitNpcDeck(t, token, registered["id"], 1)
	delete(detail, "__typename")
	require.Equal(t, node, detail)
}

func TestNpcDecks_FiltersByStatusAndRejectsEmptyStatuses(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	registered := registerNpcDeck(t, token, "初心者-"+id)
	waitNpcDeck(t, token, registered["id"], 1)

	draft := client.Data(t, token, npcDecksListQuery, map[string]any{"filter": map[string]any{"nameContains": id, "statuses": []any{"DRAFT"}}}, "npcDecks")
	published := client.Data(t, token, npcDecksListQuery, map[string]any{"filter": map[string]any{"nameContains": id, "statuses": []any{"PUBLISHED", "RETIRED"}}}, "npcDecks")
	empty := client.Data(t, token, npcDecksListQuery, map[string]any{"filter": map[string]any{"statuses": []any{}}}, "npcDecks")

	require.Len(t, draft["edges"], 1)
	require.Empty(t, published["edges"])
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldRequired", "field": "filter.statuses"},
	}}, empty)
}

// 写すのはカードと枚数で、名前は指定したもの。元の公開 ID と、写したときの revision を覚え、元が変わる・消えると、それが分かる
func TestRegisterNpcDeckFromPresetDeck_CopiesPresetDeckAndTracksSource(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	dragonID := registerPublishedCreature(t, token, "ドラゴン-"+id)
	potionID := registerPublishedItem(t, token, "ポーション-"+id)
	preset := registerPresetDeck(t, token, "火-"+id)
	filled := setPresetDeckCardCounts(t, token, preset, cardCount(dragonID, 3), cardCount(potionID, 2))
	presetDetail := waitPresetDeck(t, token, filled["id"], 2)

	copied := npcDeckOperation(t, token, registerNpcDeckFromPresetDeckMutation, "registerNpcDeckFromPresetDeck", map[string]any{"presetDeckId": preset["id"], "name": "炎-" + id})

	require.Equal(t, "RegisterNpcDeckFromPresetDeckSuccess", copied["__typename"])
	require.EqualValues(t, 1, copied["revision"])
	detail := waitNpcDeck(t, token, copied["id"], 1)
	require.Equal(t, "炎-"+id, detail["name"])
	require.Equal(t, "DRAFT", detail["status"])
	require.Equal(t, presetDetail["cards"], detail["cards"])
	require.EqualValues(t, 5, detail["totalCount"])
	require.Equal(t, map[string]any{"presetDeckId": preset["id"], "revision": float64(2), "changedSinceCopied": false, "presetDeckDeleted": false}, detail["source"])

	renamed := client.Data(t, token, renamePresetDeckMutation, map[string]any{"input": map[string]any{"id": preset["id"], "revision": 2, "name": "水-" + id}}, "renamePresetDeck")
	require.Equal(t, "RenamePresetDeckSuccess", renamed["__typename"])
	testhelper.Eventually(t, func(c *assert.CollectT) {
		detail, err := client.TryData(t.Context(), token, npcDeckQuery, map[string]any{"id": copied["id"]}, "npcDeck")
		if assert.NoError(c, err) {
			assert.Equal(c, true, detail["source"].(map[string]any)["changedSinceCopied"])
		}
	})
	deleted := client.Data(t, token, deletePresetDeckMutation, map[string]any{"input": map[string]any{"id": preset["id"], "revision": 3}}, "deletePresetDeck")
	require.Equal(t, "DeletePresetDeckSuccess", deleted["__typename"])
	testhelper.Eventually(t, func(c *assert.CollectT) {
		detail, err := client.TryData(t.Context(), token, npcDeckQuery, map[string]any{"id": copied["id"]}, "npcDeck")
		if assert.NoError(c, err) {
			source := detail["source"].(map[string]any)
			assert.Equal(c, true, source["presetDeckDeleted"])
			assert.Equal(c, false, source["changedSinceCopied"])
		}
	})
}

// 名前は元から写さないので、同じプリセットデッキから名前を変えれば何度でも作れる。使われている名前・空の名前は誤りになる
func TestRegisterNpcDeckFromPresetDeck_ReturnsErrorForUnknownPresetDeckOrInvalidName(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	preset := registerPresetDeck(t, token, "火-"+id)
	copyAs := func(name string) map[string]any {
		return npcDeckOperation(t, token, registerNpcDeckFromPresetDeckMutation, "registerNpcDeckFromPresetDeck", map[string]any{"presetDeckId": preset["id"], "name": name})
	}
	first := copyAs("火-" + id)
	require.Equal(t, "RegisterNpcDeckFromPresetDeckSuccess", first["__typename"])

	second := copyAs("火2-" + id)
	again := copyAs("火-" + id)
	blank := copyAs(" ")
	unknown := npcDeckOperation(t, token, registerNpcDeckFromPresetDeckMutation, "registerNpcDeckFromPresetDeck", map[string]any{"presetDeckId": uuid.NewString(), "name": "炎-" + id})

	require.Equal(t, "RegisterNpcDeckFromPresetDeckSuccess", second["__typename"])
	require.Equal(t, "NpcDeckNameAlreadyInUseError", again["__typename"])
	require.Equal(t, "ValidationErrors", blank["__typename"])
	require.Equal(t, []any{map[string]any{"__typename": "FieldRequired", "field": "name"}}, blank["errors"])
	require.Equal(t, "PresetDeckNotFoundError", unknown["__typename"])
}

// 名前は状態によらず、すべての NPC のデッキの中で一意。削除すると使えるようになる
func TestNpcDeckName_IsUniqueAmongAllNpcDecksRegardlessOfStatus(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	published := fullNpcDeck(t, token, "公開-"+id)
	publishedResult := publishNpcDeck(t, token, published)
	require.Equal(t, "PublishNpcDeckSuccess", publishedResult["__typename"])
	draft := registerNpcDeck(t, token, "準備中-"+id)

	sameAsPublished := npcDeckOperation(t, token, registerNpcDeckMutation, "registerNpcDeck", map[string]any{"name": " 公開-" + id})
	renameToPublished := npcDeckOperation(t, token, renameNpcDeckMutation, "renameNpcDeck", npcDeckInput(draft, map[string]any{"name": "公開-" + id}))
	retired := retireNpcDeck(t, token, ref(publishedResult))
	sameAsRetired := npcDeckOperation(t, token, registerNpcDeckMutation, "registerNpcDeck", map[string]any{"name": "公開-" + id})
	deleted := npcDeckOperation(t, token, deleteNpcDeckMutation, "deleteNpcDeck", npcDeckInput(draft, nil))
	reused := registerNpcDeck(t, token, "準備中-"+id)

	require.Equal(t, "NpcDeckNameAlreadyInUseError", sameAsPublished["__typename"])
	require.Equal(t, "NpcDeckNameAlreadyInUseError", renameToPublished["__typename"])
	require.Equal(t, "RetireNpcDeckSuccess", retired["__typename"])
	require.Equal(t, "NpcDeckNameAlreadyInUseError", sameAsRetired["__typename"])
	require.Equal(t, "DeleteNpcDeckSuccess", deleted["__typename"])
	require.Equal(t, "RegisterNpcDeckSuccess", reused["__typename"])
}

// スターターデッキ・プリセットデッキとは別の名前の集まりで、同じ名前でもよい
func TestRegisterNpcDeck_AllowsNameUsedByStarterDeckAndPresetDeck(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	registerStarterDeck(t, token, "共通-"+id)
	registerPresetDeck(t, token, "共通-"+id)

	registered := npcDeckOperation(t, token, registerNpcDeckMutation, "registerNpcDeck", map[string]any{"name": "共通-" + id})

	require.Equal(t, "RegisterNpcDeckSuccess", registered["__typename"])
}

func TestSetNpcDeckCardCounts_ShowsCardsInCardListOrder(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	spellID := registerPublishedSpell(t, token, "ファイアボール-"+id)
	dragonID := registerPublishedCreature(t, token, "ドラゴン-"+id)
	npcDeck := registerNpcDeck(t, token, "初心者-"+id)

	result := setNpcDeckCardCounts(t, token, npcDeck, cardCount(spellID, 1), cardCount(dragonID, 3))

	require.Equal(t, map[string]any{"__typename": "SetNpcDeckCardCountsSuccess", "id": npcDeck["id"], "revision": float64(2)}, result)
	detail := waitNpcDeck(t, token, npcDeck["id"], 2)
	require.Equal(t, []any{
		map[string]any{"count": float64(3), "card": map[string]any{"__typename": "Creature", "id": dragonID, "name": "ドラゴン-" + id}},
		map[string]any{"count": float64(1), "card": map[string]any{"__typename": "Spell", "id": spellID, "name": "ファイアボール-" + id}},
	}, detail["cards"])
}

// 誤りは位置を含む項目名で返し、どれか 1 つでも誤りがあれば何も変えない
func TestSetNpcDeckCardCounts_ReturnsFieldErrorPerCard(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	published := registerPublishedCreature(t, token, "ドラゴン-"+id)
	npcDeck := registerNpcDeck(t, token, "初心者-"+id)

	result := setNpcDeckCardCounts(t, token, npcDeck,
		cardCount(published, 2), cardCount(uuid.NewString(), 1), cardCount(published, 5))

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldNotFound", "field": "cards.1.cardId"},
		map[string]any{"__typename": "FieldDuplicated", "field": "cards.2.cardId"},
		map[string]any{"__typename": "FieldOutOfRange", "field": "cards.2.count", "min": float64(0), "max": float64(4)},
	}}, result)
}

// 新しく入れるカードは公開中だけ（準備中のカードは入れられない）
func TestSetNpcDeckCardCounts_ReturnsUnavailableForCardNotPublished(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	draftCard := registerCreature(t, token, "下書き-"+id)["creature"].(map[string]any)["id"]
	npcDeck := registerNpcDeck(t, token, "初心者-"+id)

	result := setNpcDeckCardCounts(t, token, npcDeck, cardCount(draftCard, 1))

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldUnavailable", "field": "cards.0.cardId"},
	}}, result)
}

// 公開すると、準備中だけの操作（名前・枚数の変更と削除）はできなくなる。使用不可でも同じ
func TestNpcDeckEditing_IsRejectedUnlessDraft(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	npcDeck := fullNpcDeck(t, token, "初心者-"+id)
	published := publishNpcDeck(t, token, npcDeck)
	require.Equal(t, "PublishNpcDeckSuccess", published["__typename"])
	cardID := registerPublishedCreature(t, token, "ドラゴン-"+id)
	states := []struct {
		name    string
		npcDeck func() map[string]any
	}{
		{name: "published", npcDeck: func() map[string]any { return ref(published) }},
		{name: "retired", npcDeck: func() map[string]any { return ref(retireNpcDeck(t, token, ref(published))) }},
	}
	for _, s := range states {
		t.Run(s.name, func(t *testing.T) {
			target := s.npcDeck()
			tests := []struct {
				name     string
				mutation string
				field    string
				extra    map[string]any
			}{
				{name: "rename", mutation: renameNpcDeckMutation, field: "renameNpcDeck", extra: map[string]any{"name": "別名-" + id}},
				{name: "set_card_counts", mutation: setNpcDeckCardCountsMutation, field: "setNpcDeckCardCounts", extra: map[string]any{"cards": []any{cardCount(cardID, 1)}}},
				{name: "delete", mutation: deleteNpcDeckMutation, field: "deleteNpcDeck"},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					result := npcDeckOperation(t, token, tt.mutation, tt.field, npcDeckInput(target, tt.extra))

					require.Equal(t, "NpcDeckNotDraftError", result["__typename"])
				})
			}
		})
	}
}

// 公開するとき確かめるのは、合計がちょうど 40 枚・入っているカードがすべて公開中。どちらも ValidationErrors の項目 cards
func TestPublishNpcDeck_ReturnsFieldErrorsWhenTotalOrCardsAreInvalid(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	short := registerNpcDeck(t, token, "少ない-"+id)
	shortFilled := setNpcDeckCardCounts(t, token, short, cardCount(registerPublishedCreature(t, token, "ドラゴン-"+id), 3))
	full := fullNpcDeck(t, token, "満杯-"+id)
	// 入っているカードを使用不可にしても残せる決まりなので、公開するときに確かめ直す
	card := client.Data(t, token, `query { cards(filter: {nameContains: "c0-満杯-`+id+`"}) { ... on CardConnection { edges { node { id revision } } } } }`, nil, "cards")
	node := card["edges"].([]any)[0].(map[string]any)["node"].(map[string]any)
	retired := retireCard(t, token, node["id"], node["revision"])
	require.Equal(t, "RetireCardSuccess", retired["__typename"])

	shortResult := publishNpcDeck(t, token, ref(shortFilled))
	fullResult := publishNpcDeck(t, token, full)

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldTotalMismatch", "field": "cards", "required": float64(40)},
	}}, shortResult)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldUnavailable", "field": "cards"},
	}}, fullResult)
}

// 公開中の数に上限は無い。使用不可にしてもう一度公開できる。同じ状態への操作は変わらない
func TestPublishNpcDeck_PublishesRetiresAndPublishesAgainWithoutLimit(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	first := fullNpcDeck(t, token, "一-"+id)
	second := fullNpcDeck(t, token, "二-"+id)
	third := fullNpcDeck(t, token, "三-"+id)
	draftRetire := retireNpcDeck(t, token, first)
	require.Equal(t, "InvalidNpcDeckStatusTransitionError", draftRetire["__typename"])

	firstPublished := publishNpcDeck(t, token, first)
	secondPublished := publishNpcDeck(t, token, second)
	thirdPublished := publishNpcDeck(t, token, third)
	samePublished := publishNpcDeck(t, token, ref(firstPublished))
	retired := retireNpcDeck(t, token, ref(firstPublished))
	sameRetired := retireNpcDeck(t, token, ref(retired))
	republished := publishNpcDeck(t, token, ref(retired))

	require.Equal(t, map[string]any{"__typename": "PublishNpcDeckSuccess", "id": first["id"], "revision": float64(3)}, firstPublished)
	require.Equal(t, "PublishNpcDeckSuccess", secondPublished["__typename"])
	require.Equal(t, "PublishNpcDeckSuccess", thirdPublished["__typename"])
	require.Equal(t, firstPublished, samePublished)
	require.Equal(t, map[string]any{"__typename": "RetireNpcDeckSuccess", "id": first["id"], "revision": float64(4)}, retired)
	require.Equal(t, retired, sameRetired)
	require.Equal(t, map[string]any{"__typename": "PublishNpcDeckSuccess", "id": first["id"], "revision": float64(5)}, republished)
	require.Equal(t, "PUBLISHED", waitNpcDeck(t, token, third["id"], 3)["status"])
}

func TestNpcDeckMutations_ReturnRevisionConflictForStaleRevision(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	stale := registerNpcDeck(t, token, "初心者-"+id)
	renamed := npcDeckOperation(t, token, renameNpcDeckMutation, "renameNpcDeck", npcDeckInput(stale, map[string]any{"name": "中級者-" + id}))
	require.Equal(t, "RenameNpcDeckSuccess", renamed["__typename"])
	cardID := registerPublishedCreature(t, token, "ドラゴン-"+id)
	tests := []struct {
		name     string
		mutation string
		field    string
		extra    map[string]any
	}{
		{name: "rename", mutation: renameNpcDeckMutation, field: "renameNpcDeck", extra: map[string]any{"name": "上級者-" + id}},
		{name: "set_card_counts", mutation: setNpcDeckCardCountsMutation, field: "setNpcDeckCardCounts", extra: map[string]any{"cards": []any{cardCount(cardID, 1)}}},
		{name: "publish", mutation: publishNpcDeckMutation, field: "publishNpcDeck"},
		{name: "retire", mutation: retireNpcDeckMutation, field: "retireNpcDeck"},
		{name: "delete", mutation: deleteNpcDeckMutation, field: "deleteNpcDeck"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := npcDeckOperation(t, token, tt.mutation, tt.field, npcDeckInput(stale, tt.extra))

			require.Equal(t, "RevisionConflictError", result["__typename"])
		})
	}
}

func TestDeleteNpcDeck_RemovesDraftNpcDeck(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	npcDeck := registerNpcDeck(t, token, "初心者-"+testhelper.UniqueID(t))
	waitNpcDeck(t, token, npcDeck["id"], 1)

	result := npcDeckOperation(t, token, deleteNpcDeckMutation, "deleteNpcDeck", npcDeckInput(npcDeck, nil))

	require.Equal(t, map[string]any{"__typename": "DeleteNpcDeckSuccess", "id": npcDeck["id"]}, result)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		found, err := client.TryData(t.Context(), token, npcDeckQuery, map[string]any{"id": npcDeck["id"]}, "npcDeck")
		if assert.NoError(c, err) {
			assert.Equal(c, "NpcDeckNotFoundError", found["__typename"])
		}
	})
}

func TestNpcDeckOperations_ReturnResultTypeWhenNotFoundOrNotOperator(t *testing.T) {
	t.Parallel()
	notOperator := key.Sign(t, idptest.ValidClaims("not-operator"))
	unknown := map[string]any{"id": uuid.NewString(), "revision": 1}
	withExtra := func(extra map[string]any) map[string]any {
		return npcDeckInput(unknown, extra)
	}
	tests := []struct {
		name      string
		token     string
		operation string
		field     string
		variables map[string]any
		want      string
	}{
		{name: "detail_of_unknown", token: operators.Token(t), operation: npcDeckQuery, field: "npcDeck",
			variables: map[string]any{"id": uuid.NewString()}, want: "NpcDeckNotFoundError"},
		{name: "rename_unknown", token: operators.Token(t), operation: renameNpcDeckMutation, field: "renameNpcDeck",
			variables: map[string]any{"input": withExtra(map[string]any{"name": "初心者"})}, want: "NpcDeckNotFoundError"},
		{name: "set_card_counts_unknown", token: operators.Token(t), operation: setNpcDeckCardCountsMutation, field: "setNpcDeckCardCounts",
			variables: map[string]any{"input": withExtra(map[string]any{"cards": []any{}})}, want: "NpcDeckNotFoundError"},
		{name: "delete_unknown", token: operators.Token(t), operation: deleteNpcDeckMutation, field: "deleteNpcDeck",
			variables: map[string]any{"input": unknown}, want: "NpcDeckNotFoundError"},
		{name: "publish_unknown", token: operators.Token(t), operation: publishNpcDeckMutation, field: "publishNpcDeck",
			variables: map[string]any{"input": unknown}, want: "NpcDeckNotFoundError"},
		{name: "retire_unknown", token: operators.Token(t), operation: retireNpcDeckMutation, field: "retireNpcDeck",
			variables: map[string]any{"input": unknown}, want: "NpcDeckNotFoundError"},
		{name: "list_by_not_operator", token: notOperator, operation: npcDecksListQuery, field: "npcDecks",
			variables: map[string]any{}, want: "ForbiddenError"},
		{name: "register_by_not_operator", token: notOperator, operation: registerNpcDeckMutation, field: "registerNpcDeck",
			variables: map[string]any{"input": map[string]any{"name": "初心者-" + testhelper.UniqueID(t)}}, want: "ForbiddenError"},
		{name: "register_from_preset_deck_by_not_operator", token: notOperator, operation: registerNpcDeckFromPresetDeckMutation, field: "registerNpcDeckFromPresetDeck",
			variables: map[string]any{"input": map[string]any{"presetDeckId": uuid.NewString(), "name": "初心者-" + testhelper.UniqueID(t)}}, want: "ForbiddenError"},
		{name: "publish_by_not_operator", token: notOperator, operation: publishNpcDeckMutation, field: "publishNpcDeck",
			variables: map[string]any{"input": unknown}, want: "ForbiddenError"},
		{name: "retire_by_not_operator", token: notOperator, operation: retireNpcDeckMutation, field: "retireNpcDeck",
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
