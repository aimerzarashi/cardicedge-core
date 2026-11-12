package studio_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

// 認証のテストは、入口を通ったかを __typename で見る（オペレーターでなければ ForbiddenError、オペレーターなら GrantDeckConnection）
const grantDecksQuery = `query {
	grantDecks {
		__typename
	}
}`

const registerCreatureMutation = `mutation ($input: RegisterCreatureInput!) {
	registerCreature(input: $input) {
		__typename
		... on RegisterCreatureSuccess { creature { id name status revision registeredAt updatedAt } }
		... on ValidationErrors { errors { __typename field ... on FieldTooLong { max } } }
	}
}`

const registerItemMutation = `mutation ($input: RegisterItemInput!) {
	registerItem(input: $input) {
		__typename
		... on RegisterItemSuccess { item { id name status category attackModifier hitPointsModifier revision registeredAt updatedAt } }
		... on ValidationErrors { errors { __typename field ... on FieldTooLong { max } } }
	}
}`

const registerSpellMutation = `mutation ($input: RegisterSpellInput!) {
	registerSpell(input: $input) {
		__typename
		... on RegisterSpellSuccess { spell { id name status revision registeredAt updatedAt } }
		... on ValidationErrors { errors { __typename field ... on FieldTooLong { max } } }
	}
}`

const presetDeckFields = `id name totalCount revision registeredAt updatedAt cards { count card { __typename id name } }`

const presetDeckQuery = `query ($id: UUID!) {
	presetDeck(id: $id) {
		__typename
		... on PresetDeck { ` + presetDeckFields + ` }
	}
}`

const registerPresetDeckMutation = `mutation ($input: RegisterPresetDeckInput!) {
	registerPresetDeck(input: $input) {
		__typename
		... on RegisterPresetDeckSuccess { id revision }
	}
}`

const renamePresetDeckMutation = `mutation ($input: RenamePresetDeckInput!) {
	renamePresetDeck(input: $input) {
		__typename
		... on RenamePresetDeckSuccess { id revision }
	}
}`

const setPresetDeckCardCountsMutation = `mutation ($input: SetPresetDeckCardCountsInput!) {
	setPresetDeckCardCounts(input: $input) {
		__typename
		... on SetPresetDeckCardCountsSuccess { id revision }
		... on ValidationErrors { errors { __typename field ... on FieldOutOfRange { min max } ... on FieldTotalTooLarge { max } } }
	}
}`

const deletePresetDeckMutation = `mutation ($input: DeletePresetDeckInput!) {
	deletePresetDeck(input: $input) {
		__typename
		... on DeletePresetDeckSuccess { id }
	}
}`

func registerCreature(t *testing.T, token, name string) map[string]any {
	t.Helper()
	return client.Data(t, token, registerCreatureMutation, map[string]any{"input": map[string]any{"name": name}}, "registerCreature")
}

func registerItem(t *testing.T, token, name string) map[string]any {
	t.Helper()
	return client.Data(t, token, registerItemMutation, map[string]any{"input": map[string]any{"name": name}}, "registerItem")
}

func registerSpell(t *testing.T, token, name string) map[string]any {
	t.Helper()
	return client.Data(t, token, registerSpellMutation, map[string]any{"input": map[string]any{"name": name}}, "registerSpell")
}

const publishCardMutation = `mutation ($input: PublishCardInput!) {
	publishCard(input: $input) {
		__typename
		... on PublishCardSuccess { id revision }
	}
}`

const retireCardMutation = `mutation ($input: RetireCardInput!) {
	retireCard(input: $input) {
		__typename
		... on RetireCardSuccess { id revision }
	}
}`

func publishCard(t *testing.T, token string, id, revision any) map[string]any {
	t.Helper()
	return client.Data(t, token, publishCardMutation, map[string]any{"input": map[string]any{"id": id, "revision": revision}}, "publishCard")
}

func retireCard(t *testing.T, token string, id, revision any) map[string]any {
	t.Helper()
	return client.Data(t, token, retireCardMutation, map[string]any{"input": map[string]any{"id": id, "revision": revision}}, "retireCard")
}

// プリセットデッキに入れられるカード（公開中）を、登録して公開して作り、公開用の ID を返す
func registerPublishedCreature(t *testing.T, token, name string) any {
	t.Helper()
	registered := registerCreature(t, token, name)["creature"].(map[string]any)
	require.Equal(t, "PublishCardSuccess", publishCard(t, token, registered["id"], registered["revision"])["__typename"])
	return registered["id"]
}

func registerPublishedItem(t *testing.T, token, name string) any {
	t.Helper()
	registered := registerItem(t, token, name)["item"].(map[string]any)
	require.Equal(t, "PublishCardSuccess", publishCard(t, token, registered["id"], registered["revision"])["__typename"])
	return registered["id"]
}

func registerPublishedSpell(t *testing.T, token, name string) any {
	t.Helper()
	registered := registerSpell(t, token, name)["spell"].(map[string]any)
	// 効果の無いスペルは公開できないので、先に効果を入れる（revision は 2 になり、公開して 3）
	effective := changeSpellEffect(t, token, registered["id"], registered["revision"], map[string]any{"drawCards": map[string]any{"count": 1}})
	require.Equal(t, "PublishCardSuccess", publishCard(t, token, registered["id"], effective["revision"])["__typename"])
	return registered["id"]
}

func registerPresetDeck(t *testing.T, token, name string) map[string]any {
	t.Helper()
	result := client.Data(t, token, registerPresetDeckMutation, map[string]any{"input": map[string]any{"name": name}}, "registerPresetDeck")
	require.Equal(t, "RegisterPresetDeckSuccess", result["__typename"])
	return result
}

func setPresetDeckCardCounts(t *testing.T, token string, deck map[string]any, cards ...map[string]any) map[string]any {
	t.Helper()
	return client.Data(t, token, setPresetDeckCardCountsMutation, map[string]any{"input": map[string]any{
		"id": deck["id"], "revision": deck["revision"], "cards": cards,
	}}, "setPresetDeckCardCounts")
}

func cardCount(cardID any, count int) map[string]any {
	return map[string]any{"cardId": cardID, "count": count}
}

// 投影されるまで待ち、revision が want になった詳細を返す
func waitPresetDeck(t *testing.T, token string, id any, revision int) map[string]any {
	t.Helper()
	var deck map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		deck, err = client.TryData(t.Context(), token, presetDeckQuery, map[string]any{"id": id}, "presetDeck")
		if assert.NoError(c, err) {
			assert.Equal(c, "PresetDeck", deck["__typename"])
			assert.EqualValues(c, revision, deck["revision"])
		}
	})
	return deck
}

// 次の操作の入力に渡せるよう、{id, revision} に直す
func ref(result map[string]any) map[string]any {
	return map[string]any{"id": result["id"], "revision": result["revision"]}
}
