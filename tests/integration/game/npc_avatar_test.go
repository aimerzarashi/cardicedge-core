package game_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const npcAvatarFields = `id name
	decks { id name totalCount cards { count card { id name type } } }`

const npcAvatarQuery = `query ($id: UUID!) {
	npcAvatar(id: $id) {
		__typename
		... on NpcAvatar { ` + npcAvatarFields + ` }
		... on NpcAvatarNotFoundError { message }
		... on PlayerNotRegisteredError { message }
	}
}`

const npcAvatarsQuery = `query ($first: Int, $after: String) {
	npcAvatars(first: $first, after: $after) {
		__typename
		... on NpcAvatarsConnection {
			edges { cursor node { ` + npcAvatarFields + ` } }
			pageInfo { hasNextPage hasPreviousPage startCursor endCursor }
		}
		... on ValidationErrors {
			errors {
				__typename
				field
				... on FieldOutOfRange { min max }
			}
		}
		... on PlayerNotRegisteredError { message }
	}
}`

func npcDeckCard(id uuid.UUID, name, cardType string, count int) map[string]any {
	return map[string]any{"count": float64(count), "card": map[string]any{"id": id.String(), "name": name, "type": cardType}}
}

// testhelper.PublishNPCAvatars が写した公開中のデッキ 1（カードは種類の順）
func npcDeckPublished1() map[string]any {
	n := master.NPCAvatars
	return map[string]any{
		"id": n.DeckPublished1.String(), "name": "公開中デッキ1", "totalCount": float64(6),
		"cards": []any{
			npcDeckCard(n.Creature, "ゴブリン", "CREATURE", 3),
			npcDeckCard(n.Item, "あいてむ", "ITEM", 2),
			npcDeckCard(n.Spell, "ファイア", "SPELL", 1),
		},
	}
}

func npcDeckPublished2() map[string]any {
	n := master.NPCAvatars
	return map[string]any{
		"id": n.DeckPublished2.String(), "name": "公開中デッキ2", "totalCount": float64(4),
		"cards": []any{npcDeckCard(n.Creature, "ゴブリン", "CREATURE", 4)},
	}
}

// 割り当てた順に 公開中のデッキ 2・準備中・公開中のデッキ 1・使用不可 のうち、公開中のものだけ
func publishedNPCAvatarA() map[string]any {
	return map[string]any{
		"id": master.NPCAvatars.PublishedA.String(), "name": "公開中A",
		"decks": []any{npcDeckPublished2(), npcDeckPublished1()},
	}
}

func publishedNPCAvatarB() map[string]any {
	return map[string]any{
		"id": master.NPCAvatars.PublishedB.String(), "name": "公開中B",
		"decks": []any{npcDeckPublished1()},
	}
}

func npcAvatarNodes(t *testing.T, result map[string]any) []any {
	t.Helper()
	require.Equal(t, "NpcAvatarsConnection", result["__typename"])
	nodes := []any{}
	for _, edge := range result["edges"].([]any) {
		nodes = append(nodes, edge.(map[string]any)["node"])
	}
	return nodes
}

// 公開は一度だけ写すので、共有の DB に入っている NPC のアバターはこの 4 つだけ
func TestNPCAvatars_ListsOnlyPublishedAvatarsInNameOrderWithPublishedDecksInAssignedOrder(t *testing.T) {
	t.Parallel()

	result := client.Data(t, registeredPlayerToken(t), npcAvatarsQuery, nil, "npcAvatars")

	require.Equal(t, []any{publishedNPCAvatarA(), publishedNPCAvatarB()}, npcAvatarNodes(t, result))
	require.Equal(t, false, result["pageInfo"].(map[string]any)["hasNextPage"])
}

func TestNPCAvatars_PagesWithCursor(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)

	first := client.Data(t, token, npcAvatarsQuery, map[string]any{"first": 1}, "npcAvatars")
	after := first["pageInfo"].(map[string]any)["endCursor"]
	second := client.Data(t, token, npcAvatarsQuery, map[string]any{"first": 1, "after": after}, "npcAvatars")

	require.Equal(t, []any{publishedNPCAvatarA()}, npcAvatarNodes(t, first))
	require.Equal(t, true, first["pageInfo"].(map[string]any)["hasNextPage"])
	require.Equal(t, []any{publishedNPCAvatarB()}, npcAvatarNodes(t, second))
	require.Equal(t, false, second["pageInfo"].(map[string]any)["hasNextPage"])
	require.Equal(t, true, second["pageInfo"].(map[string]any)["hasPreviousPage"])
}

func TestNPCAvatars_ReturnsValidationErrorsForInvalidPaging(t *testing.T) {
	t.Parallel()

	result := client.Data(t, registeredPlayerToken(t), npcAvatarsQuery, map[string]any{"first": 101, "after": "not-a-cursor"}, "npcAvatars")

	require.Equal(t, "ValidationErrors", result["__typename"])
	require.ElementsMatch(t, []any{
		map[string]any{"__typename": "FieldOutOfRange", "field": "first", "min": float64(1), "max": float64(100)},
		map[string]any{"__typename": "FieldInvalidFormat", "field": "after"},
	}, result["errors"])
}

func TestNPCAvatars_ReturnsPlayerNotRegisteredForUnregisteredPlayer(t *testing.T) {
	t.Parallel()

	result := client.Data(t, newPlayerToken(t), npcAvatarsQuery, nil, "npcAvatars")

	require.Equal(t, "PlayerNotRegisteredError", result["__typename"])
	require.NotEmpty(t, result["message"])
}

func TestNPCAvatar_ReturnsPublishedAvatarWithPublishedDecksInAssignedOrder(t *testing.T) {
	t.Parallel()

	result := client.Data(t, registeredPlayerToken(t), npcAvatarQuery, map[string]any{"id": master.NPCAvatars.PublishedA.String()}, "npcAvatar")

	want := publishedNPCAvatarA()
	want["__typename"] = "NpcAvatar"
	require.Equal(t, want, result)
}

// 準備中・使用不可は写されていても出さない（詳細は、無いものと同じ扱い）
func TestNPCAvatar_ReturnsNotFoundForAvatarThatIsNotPublished(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		id   uuid.UUID
	}{
		{name: "draft", id: master.NPCAvatars.Draft},
		{name: "retired", id: master.NPCAvatars.Retired},
		{name: "unknown", id: uuid.New()},
		// NPC のデッキは単独では引けない（アバターを通してだけ出す）
		{name: "deck_id", id: master.NPCAvatars.DeckPublished1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := client.Data(t, registeredPlayerToken(t), npcAvatarQuery, map[string]any{"id": tt.id.String()}, "npcAvatar")

			require.Equal(t, "NpcAvatarNotFoundError", result["__typename"])
			require.NotEmpty(t, result["message"])
		})
	}
}

func TestNPCAvatar_ReturnsPlayerNotRegisteredForUnregisteredPlayer(t *testing.T) {
	t.Parallel()

	result := client.Data(t, newPlayerToken(t), npcAvatarQuery, map[string]any{"id": master.NPCAvatars.PublishedA.String()}, "npcAvatar")

	require.Equal(t, "PlayerNotRegisteredError", result["__typename"])
	require.NotEmpty(t, result["message"])
}
