package game_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

func init() {
	roomOperations["declareRoomNext"] = struct {
		input  string
		errors []string
	}{"DeclareRoomNextInput", []string{"RoomNotFoundError", "RevisionConflictError", "RoomStatusNotAllowedError", "RoomAlreadyDeclaredNextError"}}
	roomOperations["resumeRoom"] = struct {
		input  string
		errors []string
	}{"ResumeRoomInput", []string{"RoomNotFoundError", "RevisionConflictError", "RoomStatusNotAllowedError", "RoomNotResponsibleError"}}
}

const roomNextQuery = `query ($id: UUID!) {
	room(id: $id) {
		__typename
		... on Room {
			revision status
			participants { playerId ready continues }
			matches { id number }
			match { id }
			capabilities { changeMatchRules declareReady declareNext resume close }
		}
	}
}`

const matchRoomQuery = `query ($id: UUID!) {
	match(id: $id) { __typename ... on Match { roomId number } }
}`

// 条件に合う部屋の状態になるまで待つ（対戦が終わったことを知らせるのはポリシーなので、遅れて反映される）
func waitRoomNext(t *testing.T, token, id string, condition func(map[string]any) bool) map[string]any {
	t.Helper()
	var found map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		result, err := client.TryData(t.Context(), token, roomNextQuery, map[string]any{"id": id}, "room")
		if !assert.NoError(c, err) || !assert.Equal(c, "Room", result["__typename"]) || !assert.True(c, condition(result)) {
			return
		}
		found = result
	})
	return found
}

// 対戦を最後まで進めて戦利品を受け取り、部屋の公開 ID を返す
func finishMatchAndReceiveReward(t *testing.T, p roomPlayer, matchID string) (roomID string) {
	t.Helper()
	ended := playUntilEnded(t, p, matchID)
	chosen := matchOperation(t, p.token, "chooseMatchRewardPack", map[string]any{"id": matchID, "revision": int(ended["revision"].(float64)), "pack": "default"})
	matchRevision(t, chosen, "chooseMatchRewardPack")
	testhelper.Eventually(t, func(c *assert.CollectT) {
		result, err := client.TryData(t.Context(), p.token, matchRoomQuery, map[string]any{"id": matchID}, "match")
		if !assert.NoError(c, err) || !assert.NotNil(c, result["roomId"]) {
			return
		}
		roomID = result["roomId"].(string)
	})
	return roomID
}

// 対戦が終わり戦利品を受け取ると、ポリシーが部屋を次を決める状態にする。続けると準備中に戻り、条件を変え直して 2 戦目を始められる。
// 部屋は始めた対戦を始めた順に持ち、対戦は部屋と何戦目かを返す
func TestRoom_ContinuingAfterTheMatchReturnsToPreparingAndStartsTheSecondMatch(t *testing.T) {
	t.Parallel()
	p := newRoomPlayer(t)
	first := startMatch(t, p, 1, func(rules map[string]any) { rules["roundLimit"] = 1 })
	roomID := finishMatchAndReceiveReward(t, p, first)

	deciding := waitRoomNext(t, p.token, roomID, func(r map[string]any) bool { return r["status"] == "DECIDING_NEXT" })
	revision := int(deciding["revision"].(float64))
	declared := roomOperation(t, p.token, "declareRoomNext", map[string]any{"id": roomID, "revision": revision, "continue": true})
	revision = roomRevision(t, declared, "declareRoomNext")
	preparing := waitRoomNext(t, p.token, roomID, func(r map[string]any) bool { return int(r["revision"].(float64)) == revision })

	require.Equal(t, map[string]any{"changeMatchRules": false, "declareReady": false, "declareNext": true, "resume": false, "close": true}, deciding["capabilities"])
	require.Equal(t, []any{map[string]any{"playerId": p.playerID, "ready": true, "continues": nil}}, deciding["participants"])
	require.Equal(t, "PREPARING", preparing["status"])
	require.Equal(t, []any{map[string]any{"playerId": p.playerID, "ready": false, "continues": nil}}, preparing["participants"])
	// 担当の決まりで条件を変え直せる（席の並び（人数）は公開前だけなので変えられない）
	require.Equal(t, true, preparing["capabilities"].(map[string]any)["changeMatchRules"])
	require.Equal(t, true, preparing["capabilities"].(map[string]any)["declareReady"])
	require.Equal(t, false, preparing["capabilities"].(map[string]any)["declareNext"])

	rules := p.rulesInput(roomID, revision, p.board["id"])
	rules["diceMax"] = 6
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomMatchRules", rules), "changeRoomMatchRules")
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeatChoice", map[string]any{
		"id": roomID, "revision": revision, "seat": 0, "avatarId": p.avatar["id"], "deckId": p.otherDeck,
	}), "changeRoomSeatChoice")
	roomOperation(t, p.token, "declareRoomReady", map[string]any{"id": roomID, "revision": revision})
	// matches は部屋のイベントの投影から読むが、match は対戦の集約をポリシーが作り終えるまで null なので、両方がそろうまで待つ
	second := waitRoomNext(t, p.token, roomID, func(r map[string]any) bool { return len(r["matches"].([]any)) == 2 && r["match"] != nil })

	matches := second["matches"].([]any)
	require.Equal(t, map[string]any{"id": first, "number": float64(1)}, matches[0])
	require.Equal(t, "PLAYING", second["status"])
	require.Equal(t, map[string]any{"id": matches[1].(map[string]any)["id"]}, second["match"])
	require.Equal(t, map[string]any{"id": matches[1].(map[string]any)["id"], "number": float64(2)}, matches[1])
	for _, entry := range matches {
		located := client.Data(t, p.token, matchRoomQuery, map[string]any{"id": entry.(map[string]any)["id"]}, "match")
		require.Equal(t, map[string]any{"__typename": "Match", "roomId": roomID, "number": entry.(map[string]any)["number"]}, located)
	}
}

// 終えると休止中になり、一覧に残る（閉じない）。意思表示は 1 回だけで、次を決める間でなければできない。作った人が再開すると準備中に戻る
func TestRoom_EndingAfterTheMatchPausesTheRoomAndResumingReturnsItToPreparing(t *testing.T) {
	t.Parallel()
	p := newRoomPlayer(t)
	matchID := startMatch(t, p, 1, func(rules map[string]any) { rules["roundLimit"] = 1 })
	roomID := finishMatchAndReceiveReward(t, p, matchID)

	deciding := waitRoomNext(t, p.token, roomID, func(r map[string]any) bool { return r["status"] == "DECIDING_NEXT" })
	revision := int(deciding["revision"].(float64))
	ended := roomOperation(t, p.token, "declareRoomNext", map[string]any{"id": roomID, "revision": revision, "continue": false})
	revision = roomRevision(t, ended, "declareRoomNext")
	paused := waitRoomNext(t, p.token, roomID, func(r map[string]any) bool { return int(r["revision"].(float64)) == revision })
	again := roomOperation(t, p.token, "declareRoomNext", map[string]any{"id": roomID, "revision": revision, "continue": true})
	var listed map[string]any
	for _, edge := range client.Data(t, p.token, myRoomsQuery, nil, "myRooms")["edges"].([]any) {
		if node := edge.(map[string]any)["node"].(map[string]any); node["id"] == roomID {
			listed = node
		}
	}
	resumed := roomOperation(t, p.token, "resumeRoom", map[string]any{"id": roomID, "revision": revision})
	revision = roomRevision(t, resumed, "resumeRoom")
	preparing := waitRoomNext(t, p.token, roomID, func(r map[string]any) bool { return int(r["revision"].(float64)) == revision })

	require.Equal(t, "PAUSED", paused["status"])
	require.Equal(t, []any{map[string]any{"playerId": p.playerID, "ready": true, "continues": false}}, paused["participants"])
	require.Equal(t, map[string]any{"changeMatchRules": false, "declareReady": false, "declareNext": false, "resume": true, "close": true}, paused["capabilities"])
	require.Equal(t, "RoomStatusNotAllowedError", again["__typename"])
	require.Equal(t, "PAUSED", listed["status"])
	require.Equal(t, "PREPARING", preparing["status"])
	require.Equal(t, []any{map[string]any{"playerId": p.playerID, "ready": false, "continues": nil}}, preparing["participants"])
	require.Equal(t, true, preparing["capabilities"].(map[string]any)["changeMatchRules"])
}

func TestRoom_DeclaringNextWhilePlayingIsNotAllowed(t *testing.T) {
	t.Parallel()
	p := newRoomPlayer(t)
	matchID := startMatch(t, p, 1, nil)
	var roomID string
	testhelper.Eventually(t, func(c *assert.CollectT) {
		result, err := client.TryData(t.Context(), p.token, matchRoomQuery, map[string]any{"id": matchID}, "match")
		if assert.NoError(c, err) && assert.NotNil(c, result["roomId"]) {
			roomID = result["roomId"].(string)
		}
	})
	playing := waitRoomNext(t, p.token, roomID, func(r map[string]any) bool { return r["status"] == "PLAYING" })

	result := roomOperation(t, p.token, "declareRoomNext", map[string]any{"id": roomID, "revision": int(playing["revision"].(float64)), "continue": true})

	require.Equal(t, "RoomStatusNotAllowedError", result["__typename"])
}
