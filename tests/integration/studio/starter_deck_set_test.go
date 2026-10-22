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

const starterDeckSetQuery = `query {
	starterDeckSet {
		__typename
		... on StarterDeckSet {
			revision updatedAt
			decks { name cards { count card { __typename id name } } source { presetDeckId revision changedSinceCopied presetDeckDeleted } }
		}
	}
}`

const setStarterDeckSetMutation = `mutation ($input: SetStarterDeckSetInput!) {
	setStarterDeckSet(input: $input) {
		__typename
		... on SetStarterDeckSetSuccess { revision }
		... on ValidationErrors { errors { __typename field ... on FieldOutOfRange { min max } ... on FieldTotalMismatch { required } } }
	}
}`

// 合計がちょうど 40 枚のプリセットデッキ（4 枚ずつ 10 種類のクリーチャー）を作り、投影されるまで待つ
func fullPresetDeck(t *testing.T, token, name string) map[string]any {
	t.Helper()
	deck := registerPresetDeck(t, token, name)
	cards := []map[string]any{}
	for i := range 10 {
		cardID := registerCreature(t, token, fmt.Sprintf("c%d-%s", i, name))["creature"].(map[string]any)["id"]
		cards = append(cards, cardCount(cardID, 4))
	}
	filled := setPresetDeckCardCounts(t, token, deck, cards...)
	return waitPresetDeck(t, token, filled["id"], 2)
}

func setStarterDeckSet(t *testing.T, token string, revision any, presetDeckIDs ...any) map[string]any {
	t.Helper()
	return client.Data(t, token, setStarterDeckSetMutation, map[string]any{"input": map[string]any{
		"revision": revision, "presetDeckIds": presetDeckIDs,
	}}, "setStarterDeckSet")
}

// 投影されるまで待ち、revision が want になったスターターデッキセットを返す
func waitStarterDeckSet(t *testing.T, token string, revision any) map[string]any {
	t.Helper()
	var set map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		set, err = client.TryData(t.Context(), token, starterDeckSetQuery, map[string]any{}, "starterDeckSet")
		if assert.NoError(c, err) {
			assert.EqualValues(c, revision, set["revision"])
		}
	})
	return set
}

// 見つからなければ nil（Eventually の中からも呼ぶため、テストを止めずに呼ぶ側で確かめる）
func deckBySource(set map[string]any, presetDeckID any) map[string]any {
	for _, deck := range set["decks"].([]any) {
		if deck.(map[string]any)["source"].(map[string]any)["presetDeckId"] == presetDeckID {
			return deck.(map[string]any)
		}
	}
	return nil
}

// スターターデッキセットはゲーム全体で 1 つなので、設定を変えるテストは並列にしない
// （ほかのテストと取り合うと、revision が合わずに「先に変えられた」になるため）
func TestSetStarterDeckSet_CopiesPresetDecksAndShowsChangesSinceCopied(t *testing.T) {
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	fireEarth := fullPresetDeck(t, token, "火・地-"+id)
	waterWind := fullPresetDeck(t, token, "水・風-"+id)
	before, err := client.TryData(t.Context(), token, starterDeckSetQuery, map[string]any{}, "starterDeckSet")
	require.NoError(t, err)
	revision := int(before["revision"].(float64))

	result := setStarterDeckSet(t, token, revision, fireEarth["id"], waterWind["id"])

	require.Equal(t, map[string]any{"__typename": "SetStarterDeckSetSuccess", "revision": float64(revision + 1)}, result)
	set := waitStarterDeckSet(t, token, revision+1)
	require.NotNil(t, set["updatedAt"])
	require.Len(t, set["decks"], 2)
	copied := deckBySource(set, fireEarth["id"])
	require.NotNil(t, copied)
	require.Equal(t, fireEarth["name"], copied["name"])
	require.Equal(t, fireEarth["cards"], copied["cards"])
	require.Equal(t, map[string]any{"presetDeckId": fireEarth["id"], "revision": fireEarth["revision"], "changedSinceCopied": false, "presetDeckDeleted": false}, copied["source"])

	// 写した後に元を直すと、写しはそのままで「変わっている」になる
	client.Data(t, token, renamePresetDeckMutation, map[string]any{"input": map[string]any{
		"id": fireEarth["id"], "revision": fireEarth["revision"], "name": "炎・大地-" + id,
	}}, "renamePresetDeck")
	testhelper.Eventually(t, func(c *assert.CollectT) {
		current, err := client.TryData(t.Context(), token, starterDeckSetQuery, map[string]any{}, "starterDeckSet")
		if assert.NoError(c, err) {
			deck := deckBySource(current, fireEarth["id"])
			if assert.NotNil(c, deck) {
				assert.Equal(c, fireEarth["name"], deck["name"])
				assert.Equal(c, true, deck["source"].(map[string]any)["changedSinceCopied"])
			}
		}
	})

	// 古い revision からの設定は先に変えられたになり、今の revision で設定し直すと反映される
	stale := setStarterDeckSet(t, token, revision, fireEarth["id"], waterWind["id"])
	require.Equal(t, "RevisionConflictError", stale["__typename"])
	recopied := setStarterDeckSet(t, token, revision+1, fireEarth["id"], waterWind["id"])
	require.Equal(t, float64(revision+2), recopied["revision"])
	reflected := deckBySource(waitStarterDeckSet(t, token, revision+2), fireEarth["id"])
	require.NotNil(t, reflected)
	require.Equal(t, "炎・大地-"+id, reflected["name"])
	require.Equal(t, false, reflected["source"].(map[string]any)["changedSinceCopied"])
}

// 写した後に元を削除すると、写しはそのままで「削除されている」になる（「変わっている」は比べる相手が無いので false）。
// 設定を変えるため並列にしない
func TestStarterDeckSet_ShowsDeletedSourcePresetDeck(t *testing.T) {
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	fireEarth := fullPresetDeck(t, token, "火・地-"+id)
	waterWind := fullPresetDeck(t, token, "水・風-"+id)
	before, err := client.TryData(t.Context(), token, starterDeckSetQuery, map[string]any{}, "starterDeckSet")
	require.NoError(t, err)
	revision := int(before["revision"].(float64))
	require.Equal(t, "SetStarterDeckSetSuccess", setStarterDeckSet(t, token, revision, fireEarth["id"], waterWind["id"])["__typename"])
	waitStarterDeckSet(t, token, revision+1)

	deleted := client.Data(t, token, deletePresetDeckMutation, map[string]any{"input": map[string]any{
		"id": fireEarth["id"], "revision": fireEarth["revision"],
	}}, "deletePresetDeck")

	require.Equal(t, "DeletePresetDeckSuccess", deleted["__typename"])
	testhelper.Eventually(t, func(c *assert.CollectT) {
		current, err := client.TryData(t.Context(), token, starterDeckSetQuery, map[string]any{}, "starterDeckSet")
		if assert.NoError(c, err) {
			deck := deckBySource(current, fireEarth["id"])
			if assert.NotNil(c, deck) {
				assert.Equal(c, fireEarth["name"], deck["name"])
				assert.Equal(c, true, deck["source"].(map[string]any)["presetDeckDeleted"])
				assert.Equal(c, false, deck["source"].(map[string]any)["changedSinceCopied"])
			}
			other := deckBySource(current, waterWind["id"])
			if assert.NotNil(c, other) {
				assert.Equal(c, false, other["source"].(map[string]any)["presetDeckDeleted"])
			}
		}
	})
}

// 入力の誤りは設定を変えないため、並列に流してよい（revision を確かめる前に入力を確かめる）
func TestSetStarterDeckSet_ReturnsFieldErrorPerReason(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	full := fullPresetDeck(t, token, "火・地-"+id)
	partial := registerPresetDeck(t, token, "作りかけ-"+id)
	tests := []struct {
		name string
		ids  []any
		want map[string]any
	}{
		{name: "one_deck", ids: []any{full["id"]}, want: map[string]any{"__typename": "FieldOutOfRange", "field": "presetDeckIds", "min": float64(2), "max": float64(2)}},
		{name: "same_deck_twice", ids: []any{full["id"], full["id"]}, want: map[string]any{"__typename": "FieldDuplicated", "field": "presetDeckIds.1"}},
		{name: "unknown_deck", ids: []any{full["id"], uuid.NewString()}, want: map[string]any{"__typename": "FieldNotFound", "field": "presetDeckIds.1"}},
		{name: "not_exactly_40", ids: []any{full["id"], partial["id"]}, want: map[string]any{"__typename": "FieldTotalMismatch", "field": "presetDeckIds.1", "required": float64(40)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := setStarterDeckSet(t, token, 0, tt.ids...)

			require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{tt.want}}, result)
		})
	}
}

func TestStarterDeckSetOperations_ReturnForbiddenForNotOperator(t *testing.T) {
	t.Parallel()
	notOperator := key.Sign(t, idptest.ValidClaims("not-operator"))
	tests := []struct {
		name      string
		operation string
		field     string
		variables map[string]any
	}{
		{name: "get", operation: starterDeckSetQuery, field: "starterDeckSet", variables: map[string]any{}},
		{name: "set", operation: setStarterDeckSetMutation, field: "setStarterDeckSet", variables: map[string]any{"input": map[string]any{
			"revision": 0, "presetDeckIds": []any{uuid.NewString(), uuid.NewString()},
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := client.Post(t, notOperator, tt.operation, tt.variables)

			require.Equal(t, http.StatusOK, status)
			require.Nil(t, body["errors"])
			require.Equal(t, "ForbiddenError", body["data"].(map[string]any)[tt.field].(map[string]any)["__typename"])
		})
	}
}
