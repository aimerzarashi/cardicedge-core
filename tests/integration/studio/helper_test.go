package studio_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

const registerCreatureMutation = `mutation ($input: RegisterCreatureInput!) {
	registerCreature(input: $input) {
		__typename
		... on RegisterCreatureSuccess { creature { id name registeredAt updatedAt } }
		... on ValidationErrors { errors { __typename field ... on FieldTooLong { max } } }
	}
}`

const registerItemMutation = `mutation ($input: RegisterItemInput!) {
	registerItem(input: $input) {
		__typename
		... on RegisterItemSuccess { item { id name registeredAt updatedAt } }
		... on ValidationErrors { errors { __typename field ... on FieldTooLong { max } } }
	}
}`

const registerSpellMutation = `mutation ($input: RegisterSpellInput!) {
	registerSpell(input: $input) {
		__typename
		... on RegisterSpellSuccess { spell { id name registeredAt updatedAt } }
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
