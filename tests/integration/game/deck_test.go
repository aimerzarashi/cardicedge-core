package game_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

const meDecksQuery = `{
	me {
		__typename
		... on Player { decks { id name totalCount typeCounts { creature item spell } cards { count card { id name type } } } }
	}
}`

// 見本のマスタのカードは名前が公開 ID、種類は Creature。同じ種類・名前の順（公開 ID の順）に並ぶ
const deckQuery = `query ($id: UUID!) {
	deck(id: $id) {
		__typename
		... on Deck { id name totalCount typeCounts { creature item spell } cards { count card { id name type } } }
		... on DeckNotFoundError { message }
		... on PlayerNotRegisteredError { message }
	}
}`

func deckCards(counts map[uuid.UUID]int) []any {
	ids := make([]uuid.UUID, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	cards := make([]any, 0, len(ids))
	for _, id := range ids {
		cards = append(cards, npcDeckCard(id, id.String(), "CREATURE", counts[id]))
	}
	return cards
}

func meDecks(t *testing.T, token string) ([]map[string]any, error) {
	t.Helper()
	result, err := client.TryData(t.Context(), token, meDecksQuery, nil, "me")
	if err != nil {
		return nil, err
	}
	decks := make([]map[string]any, 0)
	for _, deck := range result["decks"].([]any) {
		decks = append(decks, deck.(map[string]any))
	}
	return decks, nil
}

func TestMe_ListsOwnDecksWithCardsAfterStarterGrant(t *testing.T) {
	t.Parallel()
	token := newPlayerToken(t)
	registerPlayer(t, token)

	testhelper.Eventually(t, func(c *assert.CollectT) {
		decks, err := meDecks(t, token)
		if !assert.NoError(c, err) || !assert.Len(c, decks, 2) {
			return
		}
		// 配った順（公開中のスターターセットが配るデッキを指す順。名前の順ではない）に並ぶ。公開 ID はデッキごとに発行されるため、中身だけ確かめる
		assert.Equal(c, "水", decks[0]["name"])
		assert.Equal(c, float64(7), decks[0]["totalCount"])
		assert.Equal(c, deckCards(map[uuid.UUID]int{master.CardA: 2, master.CardB: 3, master.CardC: 2}), decks[0]["cards"])
		// 見本のマスタのカードはすべて Creature なので、合計が Creature の枚数になる
		assert.Equal(c, map[string]any{"creature": float64(7), "item": float64(0), "spell": float64(0)}, decks[0]["typeCounts"])
		assert.Equal(c, "火", decks[1]["name"])
		assert.Equal(c, float64(7), decks[1]["totalCount"])
		assert.Equal(c, deckCards(map[uuid.UUID]int{master.CardA: 4, master.CardB: 3}), decks[1]["cards"])
		assert.Equal(c, map[string]any{"creature": float64(7), "item": float64(0), "spell": float64(0)}, decks[1]["typeCounts"])
		assert.NotEqual(c, decks[0]["id"], decks[1]["id"])
	})
}

func TestMe_DoesNotListOtherPlayersDecks(t *testing.T) {
	t.Parallel()
	token := newPlayerToken(t)
	registerPlayer(t, token)
	otherToken := newPlayerToken(t)
	registerPlayer(t, otherToken)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		decks, err := meDecks(t, token)
		assert.NoError(c, err)
		assert.Len(c, decks, 2)
	})
	testhelper.Eventually(t, func(c *assert.CollectT) {
		decks, err := meDecks(t, otherToken)
		assert.NoError(c, err)
		assert.Len(c, decks, 2)
	})

	own, err := meDecks(t, token)
	require.NoError(t, err)
	others, err := meDecks(t, otherToken)
	require.NoError(t, err)

	ids := map[any]bool{}
	for _, deck := range own {
		ids[deck["id"]] = true
	}
	for _, deck := range others {
		require.False(t, ids[deck["id"]])
	}
}

func TestDeck_ReturnsOwnDeckWithCardsAndTypeCounts(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)
	var first map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		decks, err := meDecks(t, token)
		if !assert.NoError(c, err) || !assert.Len(c, decks, 2) {
			return
		}
		first = decks[0]
	})

	result := client.Data(t, token, deckQuery, map[string]any{"id": first["id"]}, "deck")

	require.Equal(t, "Deck", result["__typename"])
	require.Equal(t, first["id"], result["id"])
	require.Equal(t, "水", result["name"])
	require.Equal(t, float64(7), result["totalCount"])
	require.Equal(t, map[string]any{"creature": float64(7), "item": float64(0), "spell": float64(0)}, result["typeCounts"])
	require.Equal(t, deckCards(map[uuid.UUID]int{master.CardA: 2, master.CardB: 3, master.CardC: 2}), result["cards"])
}

func TestDeck_ReturnsNotFoundForOtherPlayersDeck(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)
	var ownID any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		decks, err := meDecks(t, token)
		if !assert.NoError(c, err) || !assert.NotEmpty(c, decks) {
			return
		}
		ownID = decks[0]["id"]
	})

	result := client.Data(t, registeredPlayerToken(t), deckQuery, map[string]any{"id": ownID}, "deck")

	require.Equal(t, "DeckNotFoundError", result["__typename"])
	require.NotEmpty(t, result["message"])
}

func TestDeck_ReturnsNotFoundForMissingDeck(t *testing.T) {
	t.Parallel()

	result := client.Data(t, registeredPlayerToken(t), deckQuery, map[string]any{"id": uuid.NewString()}, "deck")

	require.Equal(t, "DeckNotFoundError", result["__typename"])
}

func TestDeck_ReturnsPlayerNotRegisteredForUnregisteredPlayer(t *testing.T) {
	t.Parallel()

	result := client.Data(t, newPlayerToken(t), deckQuery, map[string]any{"id": uuid.NewString()}, "deck")

	require.Equal(t, "PlayerNotRegisteredError", result["__typename"])
}
