package game_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
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

const meNpcAvatarsQuery = `{
	me {
		... on Player { npcAvatars { ` + npcAvatarFields + ` } }
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

func meNpcAvatars(t *testing.T, token string) ([]any, error) {
	t.Helper()
	result, err := client.TryData(t.Context(), token, meNpcAvatarsQuery, nil, "me")
	if err != nil {
		return nil, err
	}
	return result["npcAvatars"].([]any), nil
}

// 解放した NPC のアバターは、スターターセットが指す順（公開中B → 公開中A）に並ぶ。セットは使用不可のアバターも指すが、公開中でないものは出ない。
// 公開中でも解放されていないアバターは出ない。デッキは割り当てた順に、公開中のものだけ
func TestMe_ListsUnlockedNPCAvatarsInUnlockOrderWithPublishedDecksInAssignedOrderAfterStarterGrant(t *testing.T) {
	t.Parallel()
	token := newPlayerToken(t)
	registerPlayer(t, token)

	// 解放がまだ届いていなくても、エラーにせず空の一覧を返す（null にしない）
	_, err := meNpcAvatars(t, token)
	require.NoError(t, err)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		avatars, err := meNpcAvatars(t, token)
		if !assert.NoError(c, err) || !assert.Len(c, avatars, 2) {
			return
		}
		assert.Equal(c, publishedNPCAvatarB(), avatars[0])
		assert.Equal(c, publishedNPCAvatarA(), avatars[1])
	})
}

// 解放したものだけ詳細で返す。解放していない（公開中でも）・準備中・使用不可・無いものは、区別せず見つからない扱い
func TestNPCAvatar_ReturnsUnlockedAvatarWithPublishedDecksInAssignedOrder(t *testing.T) {
	t.Parallel()
	token := newPlayerToken(t)
	registerPlayer(t, token)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		avatars, err := meNpcAvatars(t, token)
		assert.NoError(c, err)
		assert.Len(c, avatars, 2)
	})

	result := client.Data(t, token, npcAvatarQuery, map[string]any{"id": master.NPCAvatars.PublishedA.String()}, "npcAvatar")

	want := publishedNPCAvatarA()
	want["__typename"] = "NpcAvatar"
	require.Equal(t, want, result)
}

func TestNPCAvatar_ReturnsNotFoundForAvatarThatIsNotUnlockedOrNotPublished(t *testing.T) {
	t.Parallel()
	token := newPlayerToken(t)
	registerPlayer(t, token)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		avatars, err := meNpcAvatars(t, token)
		assert.NoError(c, err)
		assert.Len(c, avatars, 2)
	})
	tests := []struct {
		name string
		id   uuid.UUID
	}{
		{name: "published_but_not_unlocked", id: master.NPCAvatars.PublishedLocked},
		{name: "draft", id: master.NPCAvatars.Draft},
		{name: "retired_but_in_starter_set", id: master.NPCAvatars.Retired},
		{name: "unknown", id: uuid.New()},
		// NPC のデッキは単独では引けない（アバターを通してだけ出す）
		{name: "deck_id", id: master.NPCAvatars.DeckPublished1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.Data(t, token, npcAvatarQuery, map[string]any{"id": tt.id.String()}, "npcAvatar")

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
