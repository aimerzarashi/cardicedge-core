package game_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

const matchQuery = `query ($id: UUID!) {
	match(id: $id) {
		__typename
		... on Match {
			id revision round
			rules { targetMagic startingMagic lapBonus checkpointBonus roundLimit diceMin diceMax maxHandCards startingHandCards }
			board { width height tiles { __typename x y } connections { a { x y } b { x y } } pairs { a { x y } } }
			participants { kind playerId avatar { id name } position { x y } magic hand { id name type } drawPileCount turnOrder }
			currentParticipant
		}
		... on MatchNotFoundError { message }
		... on PlayerNotRegisteredError { message }
	}
}`

const roomMatchQuery = `query ($id: UUID!) {
	room(id: $id) { __typename ... on Room { status match { id } } }
}`

// 部屋の最後の準備完了で、ポリシーが部屋の条件の写しから対戦を作る。参加者のプレイヤーだけが読め、山札の並びは見せない
func TestMatch_StartsFromRoomConditionsAndIsReadableOnlyByParticipants(t *testing.T) {
	t.Parallel()
	p := newRoomPlayer(t)
	id, revision := createRoom(t, p.token)
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeats", map[string]any{"id": id, "revision": revision, "seats": []string{"PLAYER", "NPC"}}), "changeRoomSeats")
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomMatchRules", p.rulesInput(id, revision, p.board["id"])), "changeRoomMatchRules")
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeatChoice", map[string]any{
		"id": id, "revision": revision, "seat": 0, "avatarId": p.avatar["id"], "deckId": p.deck["id"],
	}), "changeRoomSeatChoice")
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeatChoice", map[string]any{
		"id": id, "revision": revision, "seat": 1, "avatarId": p.npc["id"], "deckId": p.npc["decks"].([]any)[0].(map[string]any)["id"],
	}), "changeRoomSeatChoice")
	revision = roomRevision(t, roomOperation(t, p.token, "publishRoom", map[string]any{"id": id, "revision": revision}), "publishRoom")
	roomOperation(t, p.token, "declareRoomReady", map[string]any{"id": id, "revision": revision})

	var matchID string
	testhelper.Eventually(t, func(c *assert.CollectT) {
		result, err := client.TryData(t.Context(), p.token, roomMatchQuery, map[string]any{"id": id}, "room")
		if !assert.NoError(c, err) || !assert.Equal(c, "PLAYING", result["status"]) {
			return
		}
		linked, ok := result["match"].(map[string]any)
		if !assert.True(c, ok) {
			return
		}
		matchID = linked["id"].(string)
	})
	found := client.Data(t, p.token, matchQuery, map[string]any{"id": matchID}, "match")
	stranger := newPlayerToken(t)
	registerPlayer(t, stranger)
	hidden := client.Data(t, stranger, matchQuery, map[string]any{"id": matchID}, "match")

	require.Equal(t, "Match", found["__typename"])
	require.Equal(t, matchID, found["id"])
	require.Equal(t, float64(4), found["revision"])
	require.Equal(t, float64(1), found["round"])
	require.Equal(t, map[string]any{
		"targetMagic": float64(5000), "startingMagic": float64(300), "lapBonus": float64(200), "checkpointBonus": float64(50),
		"roundLimit": float64(20), "diceMin": float64(1), "diceMax": float64(8), "maxHandCards": float64(6), "startingHandCards": float64(5),
	}, found["rules"])
	board := found["board"].(map[string]any)
	require.NotEmpty(t, board["tiles"])
	participants := found["participants"].([]any)
	require.Len(t, participants, 2)
	player := participants[0].(map[string]any)
	npc := participants[1].(map[string]any)
	require.Equal(t, "PLAYER", player["kind"])
	require.Equal(t, p.playerID, player["playerId"])
	require.Equal(t, idAndName(p.avatar), player["avatar"])
	require.Equal(t, "NPC", npc["kind"])
	require.Nil(t, npc["playerId"])
	require.Equal(t, idAndName(p.npc), npc["avatar"])
	for _, participant := range []map[string]any{player, npc} {
		require.Equal(t, float64(300), participant["magic"])
		require.Equal(t, player["position"], participant["position"])
	}
	// 山札は並びも中身も見せず枚数だけ。手札は配る枚数まで引き、デッキが足りなければある分だけ（NPC のデッキは配る枚数より少ない）
	require.Len(t, player["hand"], 5)
	require.Equal(t, float64(2), player["drawPileCount"])
	require.NotEmpty(t, npc["hand"])
	require.Equal(t, float64(0), npc["drawPileCount"])
	// 順番は 0 と 1 の並べ替えで、先頭の人の番から始まる
	require.ElementsMatch(t, []any{float64(0), float64(1)}, []any{player["turnOrder"], npc["turnOrder"]})
	first := float64(0)
	if npc["turnOrder"] == float64(0) {
		first = 1
	}
	require.Equal(t, first, found["currentParticipant"])
	require.Equal(t, "MatchNotFoundError", hidden["__typename"])
}
