package game_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

const meCollectionQuery = `query ($filter: CollectionFilter, $first: Int, $after: String) {
	me {
		__typename
		... on Player {
			collection(filter: $filter, first: $first, after: $after) {
				__typename
				... on CollectionCardConnection {
					edges { cursor node { count card { __typename id name cost rarity } } }
					pageInfo { hasNextPage hasPreviousPage endCursor }
				}
				... on ValidationErrors { errors { __typename ... on FieldError { field } } }
			}
		}
	}
}`

const cardQuery = `query ($id: UUID!) {
	card(id: $id) {
		__typename
		... on CardInfo { id name cost rarity }
		... on SpellCardInfo { effect { __typename ... on DrawCardsSpellEffect { count } } }
		... on CardNotFoundError { message }
		... on PlayerNotRegisteredError { message }
	}
}`

func meCollection(t *testing.T, token string, variables map[string]any) (map[string]any, error) {
	player, err := client.TryData(t.Context(), token, meCollectionQuery, variables, "me")
	if err != nil {
		return nil, err
	}
	return player["collection"].(map[string]any), nil
}

// 見本のマスタのカードは名前が公開 ID、種類は Creature。配布で持つ枚数は A が 4・B が 3・C が 2。同じ種類・名前の順（公開 ID の順）に並ぶ
func starterCollectionCards() []any {
	counts := map[uuid.UUID]int{master.CardA: 4, master.CardB: 3, master.CardC: 2}
	ids := make([]uuid.UUID, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	cards := make([]any, 0, len(ids))
	for _, id := range ids {
		cards = append(cards, map[string]any{"count": float64(counts[id]), "card": map[string]any{
			"__typename": "CreatureCardInfo", "id": id.String(), "name": id.String(), "cost": float64(10), "rarity": "NORMAL",
		}})
	}
	return cards
}

func collectionNodes(t *testing.T, collection map[string]any) []any {
	t.Helper()
	require.Equal(t, "CollectionCardConnection", collection["__typename"])
	nodes := []any{}
	for _, edge := range collection["edges"].([]any) {
		nodes = append(nodes, edge.(map[string]any)["node"])
	}
	return nodes
}

func TestMe_ListsOwnCollectionWithCountsAfterStarterGrant(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)
	otherToken := registeredPlayerToken(t)

	for _, owner := range []string{token, otherToken} {
		testhelper.Eventually(t, func(c *assert.CollectT) {
			collection, err := meCollection(t, owner, nil)
			if !assert.NoError(c, err) {
				return
			}
			assert.Equal(c, "CollectionCardConnection", collection["__typename"])
			assert.Equal(c, starterCollectionCards(), collectionNodes(t, collection))
			assert.Equal(c, false, collection["pageInfo"].(map[string]any)["hasNextPage"])
		})
	}
}

func TestMe_FiltersCollectionByCardType(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		collection, err := meCollection(t, token, nil)
		assert.NoError(c, err)
		assert.Len(c, collection["edges"], 3)
	})

	creatures, err := meCollection(t, token, map[string]any{"filter": map[string]any{"types": []string{"SPELL", "CREATURE"}}})
	require.NoError(t, err)
	items, err := meCollection(t, token, map[string]any{"filter": map[string]any{"types": []string{"ITEM"}}})
	require.NoError(t, err)

	require.Equal(t, starterCollectionCards(), collectionNodes(t, creatures))
	require.Empty(t, collectionNodes(t, items))
	require.Equal(t, false, items["pageInfo"].(map[string]any)["hasNextPage"])
}

func TestMe_PagesCollectionWithCursor(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		collection, err := meCollection(t, token, nil)
		assert.NoError(c, err)
		assert.Len(c, collection["edges"], 3)
	})
	all := starterCollectionCards()

	first, err := meCollection(t, token, map[string]any{"first": 2})
	require.NoError(t, err)
	firstInfo := first["pageInfo"].(map[string]any)
	second, err := meCollection(t, token, map[string]any{"first": 2, "after": firstInfo["endCursor"]})
	require.NoError(t, err)
	secondInfo := second["pageInfo"].(map[string]any)

	require.Equal(t, all[:2], collectionNodes(t, first))
	require.Equal(t, true, firstInfo["hasNextPage"])
	require.Equal(t, false, firstInfo["hasPreviousPage"])
	require.Equal(t, all[2:], collectionNodes(t, second))
	require.Equal(t, false, secondInfo["hasNextPage"])
	require.Equal(t, true, secondInfo["hasPreviousPage"])
}

func TestMe_ReturnsValidationErrorsForInvalidCollectionInput(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)

	collection, err := meCollection(t, token, map[string]any{"filter": map[string]any{"types": []string{}}, "first": 101, "after": "not-a-cursor"})

	require.NoError(t, err)
	require.Equal(t, "ValidationErrors", collection["__typename"])
	require.ElementsMatch(t, []any{
		map[string]any{"__typename": "FieldRequired", "field": "filter.types"},
		map[string]any{"__typename": "FieldOutOfRange", "field": "first"},
		map[string]any{"__typename": "FieldInvalidFormat", "field": "after"},
	}, collection["errors"])
}

func TestMe_ReturnsPlayerNotRegisteredInsteadOfCollectionForUnregisteredPlayer(t *testing.T) {
	t.Parallel()

	result := me(t, newPlayerToken(t))

	require.Equal(t, "PlayerNotRegisteredError", result["__typename"])
}

func TestCard_ReturnsPublishedCard(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)

	result := client.Data(t, token, cardQuery, map[string]any{"id": master.NPCAvatars.Item}, "card")

	require.Equal(t, map[string]any{
		"__typename": "ItemCardInfo", "id": master.NPCAvatars.Item.String(), "name": "あいてむ", "cost": float64(30), "rarity": "STRANGE",
	}, result)
}

// プレイヤーがすでに持っていることがあるので、使用不可のカードも詳細を見られる
func TestCard_ReturnsRetiredCard(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)
	id := uuid.New()
	row := models.Card{
		CardID: id.String(), CardType: 3, SortName: "しょうめつ", Status: "retired",
		Metadata: []byte(`{"id":"` + id.String() + `","name":"しょうめつ","status":"retired","cost":5,"rarity":"extra","spell":{"effect":{}}}`),
	}
	require.NoError(t, row.Insert(t.Context(), gameDB, boil.Infer()))

	result := client.Data(t, token, cardQuery, map[string]any{"id": id}, "card")

	require.Equal(t, map[string]any{"__typename": "SpellCardInfo", "id": id.String(), "name": "しょうめつ", "cost": float64(5), "rarity": "EXTRA", "effect": nil}, result)
}

// スペルの効果は種類ごとの型で返す（効果なしは null。上の使用不可のカード）
func TestCard_ReturnsTheEffectOfASpell(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)

	result := client.Data(t, token, cardQuery, map[string]any{"id": master.NPCAvatars.Spell}, "card")

	require.Equal(t, map[string]any{"__typename": "DrawCardsSpellEffect", "count": float64(2)}, result["effect"])
}

// 無い ID も、カードでない ID（登録済みのプレイヤーの ID）も、見つからない扱い
func TestCard_ReturnsNotFoundForMissingOrNonCardID(t *testing.T) {
	t.Parallel()
	token := newPlayerToken(t)
	playerID := registeredPlayerID(t, registerPlayer(t, token))

	for _, id := range []string{uuid.NewString(), playerID} {
		result := client.Data(t, token, cardQuery, map[string]any{"id": id}, "card")

		require.Equal(t, "CardNotFoundError", result["__typename"])
		require.NotEmpty(t, result["message"])
	}
}

func TestCard_ReturnsPlayerNotRegisteredForUnregisteredPlayer(t *testing.T) {
	t.Parallel()

	result := client.Data(t, newPlayerToken(t), cardQuery, map[string]any{"id": master.CardA}, "card")

	require.Equal(t, "PlayerNotRegisteredError", result["__typename"])
	require.NotEmpty(t, result["message"])
}

const creatureCardQuery = `query ($id: UUID!) {
	card(id: $id) {
		__typename
		... on CreatureCardInfo { id name elements attack hitPoints itemRestrictions placementRestrictions abilities { __typename kind ... on CreatureTollMultiplierAbility { percent } ... on CreatureBattleModifierAbility { attackModifier hitPointsModifier } } }
	}
}`

// クリーチャーは、写した cards.metadata.creature の属性・攻撃力・体力も返す
func TestCard_ReturnsCreatureElementsAttackAndHitPoints(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)

	result := client.Data(t, token, creatureCardQuery, map[string]any{"id": master.NPCAvatars.Creature}, "card")

	require.Equal(t, map[string]any{
		"__typename": "CreatureCardInfo", "id": master.NPCAvatars.Creature.String(), "name": "ゴブリン",
		"elements": []any{"FIRE", "WIND"}, "attack": float64(25), "hitPoints": float64(45), "itemRestrictions": []any{"ARMOR", "SCROLL"}, "placementRestrictions": []any{"EARTH", "WIND"}, "abilities": []any{map[string]any{"__typename": "CreatureDefensiveAbility", "kind": "DEFENSIVE"}},
	}, result)
}
