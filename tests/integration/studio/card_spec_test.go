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

const changeCardCostMutation = `mutation ($input: ChangeCardCostInput!) {
	changeCardCost(input: $input) {
		__typename
		... on ChangeCardCostSuccess { id revision }
		... on ValidationErrors { errors { __typename field ... on FieldTooSmall { min } } }
	}
}`

const changeCardRarityMutation = `mutation ($input: ChangeCardRarityInput!) {
	changeCardRarity(input: $input) {
		__typename
		... on ChangeCardRaritySuccess { id revision }
	}
}`

func changeCardCost(t *testing.T, token string, id, revision any, cost int) map[string]any {
	t.Helper()
	return client.Data(t, token, changeCardCostMutation, map[string]any{"input": map[string]any{"id": id, "revision": revision, "cost": cost}}, "changeCardCost")
}

func changeCardRarity(t *testing.T, token string, id, revision any, rarity string) map[string]any {
	t.Helper()
	return client.Data(t, token, changeCardRarityMutation, map[string]any{"input": map[string]any{"id": id, "revision": revision, "rarity": rarity}}, "changeCardRarity")
}

func TestChangeCardCostAndRarity_ChangesCardOfAnyKind(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	registered := registerEachKind(t, token, id)

	for _, card := range registered {
		costChanged := changeCardCost(t, token, card["id"], card["revision"], 30)
		require.Equal(t, map[string]any{"__typename": "ChangeCardCostSuccess", "id": card["id"], "revision": float64(2)}, costChanged)
		rarityChanged := changeCardRarity(t, token, card["id"], costChanged["revision"], "RARE")
		require.Equal(t, map[string]any{"__typename": "ChangeCardRaritySuccess", "id": card["id"], "revision": float64(3)}, rarityChanged)
	}

	testhelper.Eventually(t, func(c *assert.CollectT) {
		connection, err := client.TryData(t.Context(), token, cardsQuery, map[string]any{"filter": map[string]any{"nameContains": id}}, "cards")
		if !assert.NoError(c, err) {
			return
		}
		edges := connection["edges"].([]any)
		assert.Len(c, edges, 3)
		for _, edge := range edges {
			node := edge.(map[string]any)["node"].(map[string]any)
			assert.Equal(c, float64(30), node["cost"])
			assert.Equal(c, "RARE", node["rarity"])
			assert.Equal(c, float64(3), node["revision"])
		}
	})
}

// 同じ値への変更は成功し、revision を進めない
func TestChangeCardCostAndRarity_KeepRevisionWhenUnchanged(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	card := registerCreature(t, token, "ドラゴン-"+testhelper.UniqueID(t))["creature"].(map[string]any)

	costChanged := changeCardCost(t, token, card["id"], card["revision"], 10)
	rarityChanged := changeCardRarity(t, token, card["id"], card["revision"], "NORMAL")

	require.Equal(t, card["revision"], costChanged["revision"])
	require.Equal(t, card["revision"], rarityChanged["revision"])
}

func TestChangeCardCostAndRarity_ReturnResultTypePerReason(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	card := registerCreature(t, token, "ドラゴン-"+id)["creature"].(map[string]any)
	deck := registerPresetDeck(t, token, "初心者-"+id)
	published := registerItem(t, token, "ポーション-"+id)["item"].(map[string]any)
	publishedRevision := publishCard(t, token, published["id"], published["revision"])["revision"]
	notOperator := key.Sign(t, idptest.ValidClaims("not-operator"))
	tests := []struct {
		name     string
		mutation string
		field    string
		token    string
		input    map[string]any
		want     string
	}{
		{name: "cost_unknown_card", mutation: changeCardCostMutation, field: "changeCardCost", token: token, input: map[string]any{"id": uuid.NewString(), "revision": 1, "cost": 30}, want: "CardNotFoundError"},
		{name: "cost_not_a_card", mutation: changeCardCostMutation, field: "changeCardCost", token: token, input: map[string]any{"id": deck["id"], "revision": 1, "cost": 30}, want: "CardNotFoundError"},
		{name: "cost_stale_revision", mutation: changeCardCostMutation, field: "changeCardCost", token: token, input: map[string]any{"id": published["id"], "revision": published["revision"], "cost": 30}, want: "RevisionConflictError"},
		{name: "cost_not_draft", mutation: changeCardCostMutation, field: "changeCardCost", token: token, input: map[string]any{"id": published["id"], "revision": publishedRevision, "cost": 30}, want: "CardNotDraftError"},
		{name: "cost_negative", mutation: changeCardCostMutation, field: "changeCardCost", token: token, input: map[string]any{"id": card["id"], "revision": card["revision"], "cost": -1}, want: "ValidationErrors"},
		{name: "cost_not_operator", mutation: changeCardCostMutation, field: "changeCardCost", token: notOperator, input: map[string]any{"id": card["id"], "revision": card["revision"], "cost": 30}, want: "ForbiddenError"},
		{name: "rarity_unknown_card", mutation: changeCardRarityMutation, field: "changeCardRarity", token: token, input: map[string]any{"id": uuid.NewString(), "revision": 1, "rarity": "RARE"}, want: "CardNotFoundError"},
		{name: "rarity_stale_revision", mutation: changeCardRarityMutation, field: "changeCardRarity", token: token, input: map[string]any{"id": published["id"], "revision": published["revision"], "rarity": "RARE"}, want: "RevisionConflictError"},
		{name: "rarity_not_draft", mutation: changeCardRarityMutation, field: "changeCardRarity", token: token, input: map[string]any{"id": published["id"], "revision": publishedRevision, "rarity": "RARE"}, want: "CardNotDraftError"},
		{name: "rarity_not_operator", mutation: changeCardRarityMutation, field: "changeCardRarity", token: notOperator, input: map[string]any{"id": card["id"], "revision": card["revision"], "rarity": "RARE"}, want: "ForbiddenError"},
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

func TestChangeCardCost_ReturnsFieldTooSmallForNegative(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	card := registerCreature(t, token, "ドラゴン-"+testhelper.UniqueID(t))["creature"].(map[string]any)

	result := changeCardCost(t, token, card["id"], card["revision"], -1)

	require.Equal(t, map[string]any{
		"__typename": "ValidationErrors",
		"errors":     []any{map[string]any{"__typename": "FieldTooSmall", "field": "cost", "min": float64(0)}},
	}, result)
}

const cardQuery = `query ($id: UUID!) {
	card(id: $id) {
		__typename
		... on Card { id name status cost rarity revision }
	}
}`

func TestCard_ReturnsCardOfAnyKindAfterProjected(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	registered := registerEachKind(t, token, testhelper.UniqueID(t))
	typenames := map[string]string{"creature": "Creature", "item": "Item", "spell": "Spell"}

	for kind, card := range registered {
		changed := changeCardCost(t, token, card["id"], card["revision"], 25)
		testhelper.Eventually(t, func(c *assert.CollectT) {
			got, err := client.TryData(t.Context(), token, cardQuery, map[string]any{"id": card["id"]}, "card")
			if assert.NoError(c, err) {
				assert.Equal(c, map[string]any{
					"__typename": typenames[kind], "id": card["id"], "name": card["name"], "status": "DRAFT",
					"cost": float64(25), "rarity": "NORMAL", "revision": changed["revision"],
				}, got)
			}
		})
	}
}

func TestCard_ReturnsResultTypePerReason(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	card := registerCreature(t, token, "ドラゴン-"+id)["creature"].(map[string]any)
	deck := registerPresetDeck(t, token, "初心者-"+id)
	tests := []struct {
		name  string
		token string
		id    any
		want  string
	}{
		{name: "unknown_card", token: token, id: uuid.NewString(), want: "CardNotFoundError"},
		{name: "not_a_card", token: token, id: deck["id"], want: "CardNotFoundError"},
		{name: "not_operator", token: key.Sign(t, idptest.ValidClaims("not-operator")), id: card["id"], want: "ForbiddenError"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.Data(t, tt.token, cardQuery, map[string]any{"id": tt.id}, "card")

			require.Equal(t, tt.want, got["__typename"])
		})
	}
}

const changeCreatureElementsMutation = `mutation ($input: ChangeCreatureElementsInput!) {
	changeCreatureElements(input: $input) {
		__typename
		... on ChangeCreatureElementsSuccess { id revision }
		... on ValidationErrors { errors { __typename field } }
	}
}`

const changeCreatureStatsMutation = `mutation ($input: ChangeCreatureStatsInput!) {
	changeCreatureStats(input: $input) {
		__typename
		... on ChangeCreatureStatsSuccess { id revision }
		... on ValidationErrors { errors { __typename field ... on FieldTooSmall { min } } }
	}
}`

const creatureSpecQuery = `query ($id: UUID!) {
	card(id: $id) {
		__typename
		... on Creature { id elements attack hitPoints revision }
	}
}`

func changeCreatureElements(t *testing.T, token string, id, revision any, elements []string) map[string]any {
	t.Helper()
	return client.Data(t, token, changeCreatureElementsMutation, map[string]any{"input": map[string]any{"id": id, "revision": revision, "elements": elements}}, "changeCreatureElements")
}

func changeCreatureStats(t *testing.T, token string, id, revision any, attack, hitPoints int) map[string]any {
	t.Helper()
	return client.Data(t, token, changeCreatureStatsMutation, map[string]any{"input": map[string]any{"id": id, "revision": revision, "attack": attack, "hitPoints": hitPoints}}, "changeCreatureStats")
}

func TestChangeCreatureElementsAndStats_ChangesCreatureAfterProjected(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	card := registerCreature(t, token, "ドラゴン-"+testhelper.UniqueID(t))["creature"].(map[string]any)

	elementsChanged := changeCreatureElements(t, token, card["id"], card["revision"], []string{"WIND", "FIRE"})
	statsChanged := changeCreatureStats(t, token, card["id"], elementsChanged["revision"], 30, 50)

	require.Equal(t, map[string]any{"__typename": "ChangeCreatureElementsSuccess", "id": card["id"], "revision": float64(2)}, elementsChanged)
	require.Equal(t, map[string]any{"__typename": "ChangeCreatureStatsSuccess", "id": card["id"], "revision": float64(3)}, statsChanged)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		got, err := client.TryData(t.Context(), token, creatureSpecQuery, map[string]any{"id": card["id"]}, "card")
		if assert.NoError(c, err) {
			assert.Equal(c, map[string]any{
				"__typename": "Creature", "id": card["id"], "elements": []any{"FIRE", "WIND"},
				"attack": float64(30), "hitPoints": float64(50), "revision": float64(3),
			}, got)
		}
	})
}

func TestChangeCreatureMovement_ChangesCreatureAfterProjected(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	card := registerCreature(t, token, "ドラゴン-"+testhelper.UniqueID(t))["creature"].(map[string]any)

	changed := client.Data(t, token, `mutation ($input: ChangeCreatureMovementInput!) {
		changeCreatureMovement(input: $input) { __typename ... on ChangeCreatureMovementSuccess { id revision } ... on ValidationErrors { errors { __typename field } } }
	}`, map[string]any{"input": map[string]any{"id": card["id"], "revision": card["revision"], "movement": 0}}, "changeCreatureMovement")
	invalid := client.Data(t, token, `mutation ($input: ChangeCreatureMovementInput!) {
		changeCreatureMovement(input: $input) { __typename ... on ValidationErrors { errors { __typename field } } }
	}`, map[string]any{"input": map[string]any{"id": card["id"], "revision": changed["revision"], "movement": -1}}, "changeCreatureMovement")

	require.Equal(t, map[string]any{"__typename": "ChangeCreatureMovementSuccess", "id": card["id"], "revision": float64(2)}, changed)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{map[string]any{"__typename": "FieldTooSmall", "field": "movement"}}}, invalid)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		got, err := client.TryData(t.Context(), token, `query ($id: UUID!) { card(id: $id) { ... on Creature { movement revision } } }`, map[string]any{"id": card["id"]}, "card")
		if assert.NoError(c, err) {
			assert.Equal(c, map[string]any{"movement": float64(0), "revision": float64(2)}, got)
		}
	})
}

// 登録した直後の性能は、登録の結果にも出る
func TestRegisterCreature_ReturnsDefaultSpec(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)

	got := client.Data(t, token, `mutation ($input: RegisterCreatureInput!) {
		registerCreature(input: $input) { ... on RegisterCreatureSuccess { creature { elements attack hitPoints } } }
	}`, map[string]any{"input": map[string]any{"name": "ドラゴン-" + testhelper.UniqueID(t)}}, "registerCreature")

	require.Equal(t, map[string]any{"creature": map[string]any{"elements": []any{"NEUTRAL"}, "attack": float64(10), "hitPoints": float64(10)}}, got)
}

func TestChangeCreatureElementsAndStats_ReturnResultTypePerReason(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	card := registerCreature(t, token, "ドラゴン-"+id)["creature"].(map[string]any)
	item := registerItem(t, token, "ポーション-"+id)["item"].(map[string]any)
	published := registerCreature(t, token, "ウルフ-"+id)["creature"].(map[string]any)
	publishedRevision := publishCard(t, token, published["id"], published["revision"])["revision"]
	notOperator := key.Sign(t, idptest.ValidClaims("not-operator"))
	elements := func(token string, id, revision any, values ...string) (string, string, map[string]any, string) {
		return changeCreatureElementsMutation, "changeCreatureElements", map[string]any{"id": id, "revision": revision, "elements": append([]string{}, values...)}, token
	}
	stats := func(token string, id, revision any, attack, hitPoints int) (string, string, map[string]any, string) {
		return changeCreatureStatsMutation, "changeCreatureStats", map[string]any{"id": id, "revision": revision, "attack": attack, "hitPoints": hitPoints}, token
	}
	type call func() (mutation, field string, input map[string]any, token string)
	tests := []struct {
		name string
		call call
		want string
	}{
		{name: "elements_unknown_card", call: func() (string, string, map[string]any, string) { return elements(token, uuid.NewString(), 1, "FIRE") }, want: "CardNotFoundError"},
		{name: "elements_not_a_creature", call: func() (string, string, map[string]any, string) {
			return elements(token, item["id"], item["revision"], "FIRE")
		}, want: "CardNotFoundError"},
		{name: "elements_stale_revision", call: func() (string, string, map[string]any, string) {
			return elements(token, published["id"], published["revision"], "FIRE")
		}, want: "RevisionConflictError"},
		{name: "elements_not_draft", call: func() (string, string, map[string]any, string) {
			return elements(token, published["id"], publishedRevision, "FIRE")
		}, want: "CardNotDraftError"},
		{name: "elements_empty", call: func() (string, string, map[string]any, string) { return elements(token, card["id"], card["revision"]) }, want: "ValidationErrors"},
		{name: "elements_neutral_mixed", call: func() (string, string, map[string]any, string) {
			return elements(token, card["id"], card["revision"], "NEUTRAL", "FIRE")
		}, want: "ValidationErrors"},
		{name: "elements_duplicated", call: func() (string, string, map[string]any, string) {
			return elements(token, card["id"], card["revision"], "FIRE", "FIRE")
		}, want: "ValidationErrors"},
		{name: "elements_not_operator", call: func() (string, string, map[string]any, string) {
			return elements(notOperator, card["id"], card["revision"], "FIRE")
		}, want: "ForbiddenError"},
		{name: "stats_unknown_card", call: func() (string, string, map[string]any, string) { return stats(token, uuid.NewString(), 1, 30, 50) }, want: "CardNotFoundError"},
		{name: "stats_not_a_creature", call: func() (string, string, map[string]any, string) {
			return stats(token, item["id"], item["revision"], 30, 50)
		}, want: "CardNotFoundError"},
		{name: "stats_stale_revision", call: func() (string, string, map[string]any, string) {
			return stats(token, published["id"], published["revision"], 30, 50)
		}, want: "RevisionConflictError"},
		{name: "stats_not_draft", call: func() (string, string, map[string]any, string) {
			return stats(token, published["id"], publishedRevision, 30, 50)
		}, want: "CardNotDraftError"},
		{name: "stats_negative_attack", call: func() (string, string, map[string]any, string) {
			return stats(token, card["id"], card["revision"], -1, 50)
		}, want: "ValidationErrors"},
		{name: "stats_zero_hit_points", call: func() (string, string, map[string]any, string) {
			return stats(token, card["id"], card["revision"], 30, 0)
		}, want: "ValidationErrors"},
		{name: "stats_not_operator", call: func() (string, string, map[string]any, string) {
			return stats(notOperator, card["id"], card["revision"], 30, 50)
		}, want: "ForbiddenError"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutation, field, input, token := tt.call()

			status, body := client.Post(t, token, mutation, map[string]any{"input": input})

			require.Equal(t, http.StatusOK, status)
			require.Nil(t, body["errors"])
			require.Equal(t, tt.want, body["data"].(map[string]any)[field].(map[string]any)["__typename"])
		})
	}
}

func TestChangeCreatureStats_ReturnsFieldTooSmallPerField(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	card := registerCreature(t, token, "ドラゴン-"+testhelper.UniqueID(t))["creature"].(map[string]any)

	result := changeCreatureStats(t, token, card["id"], card["revision"], -1, 0)

	require.Equal(t, map[string]any{
		"__typename": "ValidationErrors",
		"errors": []any{
			map[string]any{"__typename": "FieldTooSmall", "field": "attack", "min": float64(0)},
			map[string]any{"__typename": "FieldTooSmall", "field": "hitPoints", "min": float64(1)},
		},
	}, result)
}

const changeItemCategoryMutation = `mutation ($input: ChangeItemCategoryInput!) {
	changeItemCategory(input: $input) {
		__typename
		... on ChangeItemCategorySuccess { id revision }
	}
}`

const changeItemStatBonusesMutation = `mutation ($input: ChangeItemStatBonusesInput!) {
	changeItemStatBonuses(input: $input) {
		__typename
		... on ChangeItemStatBonusesSuccess { id revision }
	}
}`

// 変えた分類と増減が登録の結果の既定の値を置き換え、カードの詳細で読める
func TestItem_CategoryAndStatBonusesCanBeChangedWhileDraft(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	registered := registerItem(t, token, "かぶと-"+id)["item"].(map[string]any)
	require.Equal(t, "WEAPON", registered["category"])
	require.Equal(t, float64(0), registered["attackBonus"])
	require.Equal(t, float64(0), registered["hitPointsBonus"])

	categorized := client.Data(t, token, changeItemCategoryMutation, map[string]any{"input": map[string]any{"id": registered["id"], "revision": registered["revision"], "category": "ARMOR"}}, "changeItemCategory")
	require.Equal(t, "ChangeItemCategorySuccess", categorized["__typename"])
	changed := client.Data(t, token, changeItemStatBonusesMutation, map[string]any{"input": map[string]any{"id": registered["id"], "revision": categorized["revision"], "attackBonus": -5, "hitPointsBonus": 30}}, "changeItemStatBonuses")
	require.Equal(t, "ChangeItemStatBonusesSuccess", changed["__typename"])
	require.Equal(t, registered["revision"].(float64)+2, changed["revision"])
}

func TestItem_CategoryAndStatBonusesAreRejectedForOtherThanADraftItem(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	creature := registerCreature(t, token, "ドラゴン-"+id)["creature"].(map[string]any)
	published := registerItem(t, token, "かぶと-"+id)["item"].(map[string]any)
	publishedRevision := publishCard(t, token, published["id"], published["revision"])["revision"]

	notAnItem := client.Data(t, token, changeItemCategoryMutation, map[string]any{"input": map[string]any{"id": creature["id"], "revision": creature["revision"], "category": "ARMOR"}}, "changeItemCategory")
	notDraft := client.Data(t, token, changeItemStatBonusesMutation, map[string]any{"input": map[string]any{"id": published["id"], "revision": publishedRevision, "attackBonus": 1, "hitPointsBonus": 1}}, "changeItemStatBonuses")

	require.Equal(t, "CardNotFoundError", notAnItem["__typename"])
	require.Equal(t, "CardNotDraftError", notDraft["__typename"])
}

const changeSpellEffectMutation = `mutation ($input: ChangeSpellEffectInput!) {
	changeSpellEffect(input: $input) {
		__typename
		... on ChangeSpellEffectSuccess { id revision }
		... on ValidationErrors { errors { __typename field ... on FieldTooSmall { min } } }
	}
}`

func changeSpellEffect(t *testing.T, token string, id, revision, effect any) map[string]any {
	t.Helper()
	return client.Data(t, token, changeSpellEffectMutation, map[string]any{"input": map[string]any{"id": id, "revision": revision, "effect": effect}}, "changeSpellEffect")
}

func TestChangeSpellEffect_ChangesSpellAfterProjected(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	card := registerSpell(t, token, "ホープ-"+testhelper.UniqueID(t))["spell"].(map[string]any)

	changed := changeSpellEffect(t, token, card["id"], card["revision"], map[string]any{"drawCards": map[string]any{"count": 2}})

	require.Equal(t, map[string]any{"__typename": "ChangeSpellEffectSuccess", "id": card["id"], "revision": float64(2)}, changed)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		got, err := client.TryData(t.Context(), token, `query ($id: UUID!) { card(id: $id) { ... on Spell { effect { __typename ... on SpellDrawCardsEffect { count } } revision } } }`, map[string]any{"id": card["id"]}, "card")
		if assert.NoError(c, err) {
			assert.Equal(c, map[string]any{"effect": map[string]any{"__typename": "SpellDrawCardsEffect", "count": float64(2)}, "revision": float64(2)}, got)
		}
	})
	// null は効果なしに戻す
	cleared := changeSpellEffect(t, token, card["id"], changed["revision"], nil)
	require.Equal(t, "ChangeSpellEffectSuccess", cleared["__typename"])
	testhelper.Eventually(t, func(c *assert.CollectT) {
		got, err := client.TryData(t.Context(), token, `query ($id: UUID!) { card(id: $id) { ... on Spell { effect { __typename } revision } } }`, map[string]any{"id": card["id"]}, "card")
		if assert.NoError(c, err) {
			assert.Equal(c, map[string]any{"effect": nil, "revision": float64(3)}, got)
		}
	})
}

func TestChangeSpellEffect_ReturnsResultTypePerReason(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	spell := registerSpell(t, token, "ホープ-"+id)["spell"].(map[string]any)
	creature := registerCreature(t, token, "ドラゴン-"+id)["creature"].(map[string]any)
	published := registerSpell(t, token, "フライ-"+id)["spell"].(map[string]any)
	// 効果の無いスペルは公開できないので、先に効果を入れる
	effective := changeSpellEffect(t, token, published["id"], published["revision"], map[string]any{"rollDice": map[string]any{"count": 2}})
	publishedRevision := publishCard(t, token, published["id"], effective["revision"])["revision"]

	tooSmall := changeSpellEffect(t, token, spell["id"], spell["revision"], map[string]any{"drawCards": map[string]any{"count": 0}})
	noKind := changeSpellEffect(t, token, spell["id"], spell["revision"], map[string]any{})
	twoKinds := changeSpellEffect(t, token, spell["id"], spell["revision"], map[string]any{"gainMagic": map[string]any{"perLap": 100}, "rollDice": map[string]any{"count": 2}})
	notASpell := changeSpellEffect(t, token, creature["id"], creature["revision"], nil)
	notDraft := changeSpellEffect(t, token, published["id"], publishedRevision, nil)

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{map[string]any{"__typename": "FieldTooSmall", "field": "effect.drawCards.count", "min": float64(1)}}}, tooSmall)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{map[string]any{"__typename": "FieldInvalidFormat", "field": "effect"}}}, noKind)
	require.Equal(t, noKind, twoKinds)
	require.Equal(t, "CardNotFoundError", notASpell["__typename"])
	require.Equal(t, "CardNotDraftError", notDraft["__typename"])
}

func TestChangeSpellEffect_ReturnsEachKindAfterProjected(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := testhelper.UniqueID(t)
	mana := registerSpell(t, token, "マナ-"+id)["spell"].(map[string]any)
	fly := registerSpell(t, token, "フライ-"+id)["spell"].(map[string]any)
	holyWord := registerSpell(t, token, "ホーリーワード6-"+id)["spell"].(map[string]any)

	require.Equal(t, "ChangeSpellEffectSuccess", changeSpellEffect(t, token, mana["id"], mana["revision"], map[string]any{"gainMagic": map[string]any{"perLap": 100}})["__typename"])
	require.Equal(t, "ChangeSpellEffectSuccess", changeSpellEffect(t, token, fly["id"], fly["revision"], map[string]any{"rollDice": map[string]any{"count": 2}})["__typename"])
	require.Equal(t, "ChangeSpellEffectSuccess", changeSpellEffect(t, token, holyWord["id"], holyWord["revision"], map[string]any{"fixedDiceCurse": map[string]any{"value": 6, "turns": 1}})["__typename"])

	const effectQuery = `query ($id: UUID!) { card(id: $id) { ... on Spell { effect { __typename ... on SpellGainMagicEffect { perLap } ... on SpellRollDiceEffect { count } ... on SpellFixedDiceCurseEffect { value turns } ... on SpellDamageEffect { amount } } } } }`
	testhelper.Eventually(t, func(c *assert.CollectT) {
		gotMana, err := client.TryData(t.Context(), token, effectQuery, map[string]any{"id": mana["id"]}, "card")
		if assert.NoError(c, err) {
			assert.Equal(c, map[string]any{"effect": map[string]any{"__typename": "SpellGainMagicEffect", "perLap": float64(100)}}, gotMana)
		}
		gotFly, err := client.TryData(t.Context(), token, effectQuery, map[string]any{"id": fly["id"]}, "card")
		if assert.NoError(c, err) {
			assert.Equal(c, map[string]any{"effect": map[string]any{"__typename": "SpellRollDiceEffect", "count": float64(2)}}, gotFly)
		}
		gotHolyWord, err := client.TryData(t.Context(), token, effectQuery, map[string]any{"id": holyWord["id"]}, "card")
		if assert.NoError(c, err) {
			assert.Equal(c, map[string]any{"effect": map[string]any{"__typename": "SpellFixedDiceCurseEffect", "value": float64(6), "turns": float64(1)}}, gotHolyWord)
		}
	})
}

func TestPublishCard_ReturnsSpellEffectRequiredForASpellWithoutEffect(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	spell := registerSpell(t, token, "マジックボルト-"+testhelper.UniqueID(t))["spell"].(map[string]any)

	result := client.Data(t, token, `mutation ($input: PublishCardInput!) { publishCard(input: $input) { __typename } }`, map[string]any{"input": map[string]any{"id": spell["id"], "revision": spell["revision"]}}, "publishCard")

	require.Equal(t, "SpellEffectRequiredError", result["__typename"])
}
