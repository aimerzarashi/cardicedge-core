package game_test

import (
	"fmt"
	"strings"
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
			participants { kind playerId avatar { id name } position { x y } magic hand { id name type } drawPileCount turnOrder passedCheckpoints { x y } }
			currentParticipant
			phase dice remainingSteps pathChoices { x y } outcome { winners reason }
		}
		... on MatchNotFoundError { message }
		... on PlayerNotRegisteredError { message }
	}
}`

const roomMatchQuery = `query ($id: UUID!) {
	room(id: $id) { __typename ... on Room { status match { id } } }
}`

// プレイヤー 1 人と NPC 1 人の部屋を、準備完了まで進める（ポリシーが対戦を作る）。対戦の公開 ID が部屋に付くまで待って返す。
// adjustRules は対戦ルールの入力を書き換える（nil なら見本のまま）
// 席 0 がプレイヤー、続く npcs 個の席が NPC（どの NPC の席も同じ NPC とデッキ）
func startMatch(t *testing.T, p roomPlayer, npcs int, adjustRules func(map[string]any)) string {
	t.Helper()
	id, revision := createRoom(t, p.token)
	seats := []string{"PLAYER"}
	for range npcs {
		seats = append(seats, "NPC")
	}
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeats", map[string]any{"id": id, "revision": revision, "seats": seats}), "changeRoomSeats")
	rules := p.rulesInput(id, revision, p.board["id"])
	if adjustRules != nil {
		adjustRules(rules)
	}
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomMatchRules", rules), "changeRoomMatchRules")
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeatChoice", map[string]any{
		"id": id, "revision": revision, "seat": 0, "avatarId": p.avatar["id"], "deckId": p.deck["id"],
	}), "changeRoomSeatChoice")
	for seat := 1; seat <= npcs; seat++ {
		revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeatChoice", map[string]any{
			"id": id, "revision": revision, "seat": seat, "avatarId": p.npc["id"], "deckId": p.npc["decks"].([]any)[0].(map[string]any)["id"],
		}), "changeRoomSeatChoice")
	}
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
	return matchID
}

// 部屋の最後の準備完了で、ポリシーが部屋の条件の写しから対戦を作る。参加者のプレイヤーだけが読め、山札の並びは見せない
func TestMatch_StartsFromRoomConditionsAndIsReadableOnlyByParticipants(t *testing.T) {
	t.Parallel()
	p := newRoomPlayer(t)
	matchID := startMatch(t, p, 1, nil)
	// NPC が先の順番なら、NPC のターンが済んでプレイヤーの番になるのを待つ（NPC は対戦が始まるとすぐ動く）
	found := waitMatch(t, p.token, matchID, func(m map[string]any) bool { return m["currentParticipant"] == float64(0) })
	stranger := newPlayerToken(t)
	registerPlayer(t, stranger)
	hidden := client.Data(t, stranger, matchQuery, map[string]any{"id": matchID}, "match")

	require.Equal(t, "Match", found["__typename"])
	require.Equal(t, matchID, found["id"])
	// 始めたときの 4 つのイベント以上（NPC が先に動いていれば、その分だけ進んでいる）
	require.GreaterOrEqual(t, found["revision"].(float64), float64(4))
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
	// プレイヤーはまだ動いていない（NPC は先に動いていることがあるので見ない）
	require.Equal(t, float64(300), player["magic"])
	require.Equal(t, map[string]any{"x": float64(0), "y": float64(0)}, player["position"])
	require.Empty(t, player["passedCheckpoints"])
	// 山札は並びも中身も見せず枚数だけ。手札は配る枚数まで引き、デッキが足りなければある分だけ（NPC のデッキは配る枚数より少ない）
	require.Len(t, player["hand"], 5)
	require.Equal(t, float64(2), player["drawPileCount"])
	require.NotEmpty(t, npc["hand"])
	require.Equal(t, float64(0), npc["drawPileCount"])
	// 順番は 0 と 1 の並べ替えで、先頭の人の番から始まる
	require.ElementsMatch(t, []any{float64(0), float64(1)}, []any{player["turnOrder"], npc["turnOrder"]})
	require.Equal(t, "DRAWING", found["phase"])
	require.Nil(t, found["dice"])
	require.Equal(t, float64(0), found["remainingSteps"])
	require.Empty(t, found["pathChoices"])
	require.Nil(t, found["outcome"])
	require.Equal(t, "MatchNotFoundError", hidden["__typename"])
}

// 条件に合う対戦の状態になるまで待つ（NPC の番はポリシーが進める）
func waitMatch(t *testing.T, token, matchID string, condition func(map[string]any) bool) map[string]any {
	t.Helper()
	var found map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		result, err := client.TryData(t.Context(), token, matchQuery, map[string]any{"id": matchID}, "match")
		if !assert.NoError(c, err) || !assert.Equal(c, "Match", result["__typename"]) {
			return
		}
		if assert.True(c, condition(result)) {
			found = result
		}
	})
	return found
}

func matchOperationQuery(name string) string {
	upper := strings.ToUpper(name[:1]) + name[1:]
	validation := ""
	if name == "discardMatchCard" || name == "chooseMatchPath" {
		validation = "... on ValidationErrors { errors { __typename field ... on FieldOutOfRange { min max } } }"
	}
	return fmt.Sprintf(`mutation ($input: %sInput!) {
		%s(input: $input) {
			__typename
			... on %sSuccess { id revision }
			%s
			... on MatchNotFoundError { message }
			... on MatchNotYourTurnError { message }
			... on MatchPhaseNotAllowedError { message }
			... on RevisionConflictError { message }
			... on PlayerNotRegisteredError { message }
		}
	}`, upper, name, upper, validation)
}

func matchOperation(t *testing.T, token, name string, input map[string]any) map[string]any {
	t.Helper()
	return client.Data(t, token, matchOperationQuery(name), map[string]any{"input": input}, name)
}

// 成功を確かめ、新しい revision を返す
func matchRevision(t *testing.T, result map[string]any, name string) int {
	t.Helper()
	require.Equal(t, strings.ToUpper(name[:1])+name[1:]+"Success", result["__typename"])
	return int(result["revision"].(float64))
}

// プレイヤーの番で、段階に合う操作を 1 つする。ドメインの決まり（来たマスには戻らない等）を画面に持たせないため、
// 分かれ道は core が出す pathChoices から選ぶ。次の revision を返す
func playMatchPhase(t *testing.T, token, matchID string, m map[string]any) int {
	t.Helper()
	revision := int(m["revision"].(float64))
	input := map[string]any{"id": matchID, "revision": revision}
	switch m["phase"] {
	case "DRAWING":
		return matchRevision(t, matchOperation(t, token, "drawMatchCard", input), "drawMatchCard")
	case "DISCARDING":
		input["handIndex"] = 0
		return matchRevision(t, matchOperation(t, token, "discardMatchCard", input), "discardMatchCard")
	case "ROLLING":
		return matchRevision(t, matchOperation(t, token, "rollMatchDice", input), "rollMatchDice")
	case "CHOOSING_PATH":
		choices := m["pathChoices"].([]any)
		require.NotEmpty(t, choices)
		input["to"] = choices[0]
		return matchRevision(t, matchOperation(t, token, "chooseMatchPath", input), "chooseMatchPath")
	case "ENDING":
		return matchRevision(t, matchOperation(t, token, "endMatchTurn", input), "endMatchTurn")
	}
	require.FailNow(t, "unexpected phase")
	return 0
}

// プレイヤーの番は GraphQL の操作で、NPC の番はポリシーに任せて、ラウンドの上限で終わるまで最後まで進める
func TestMatch_PlayerAndNPCPlayTheMatchToTheEndThroughGraphQL(t *testing.T) {
	t.Parallel()
	p := newRoomPlayer(t)
	// NPC を 2 人にし、3 人の順番の中で NPC の番が必ず続くようにする（NPC だけが続く番もポリシーで進むこと）
	matchID := startMatch(t, p, 2, func(rules map[string]any) { rules["roundLimit"] = 2 })
	const playerSeat = float64(0)

	stranger := newPlayerToken(t)
	registerPlayer(t, stranger)
	for step := 0; ; step++ {
		require.Less(t, step, 300)
		m := waitMatch(t, p.token, matchID, func(m map[string]any) bool {
			return m["phase"] == "ENDED" || m["currentParticipant"] == playerSeat
		})
		if m["phase"] == "ENDED" {
			break
		}
		assertOperationsAreRejected(t, p.token, stranger, matchID, m)
		next := playMatchPhase(t, p.token, matchID, m)
		if m["phase"] != "ENDING" {
			continue
		}
		// ターンを終えた直後は NPC の番（上限のラウンドの最後のプレイヤーなら対戦が終わっている）。番でない操作は断られ、何も変わらない。
		// NPC は終えた直後から動くので、先に動いていれば revision の食い違いで断られる（どちらも断られたことに変わりはない）
		result := matchOperation(t, p.token, "drawMatchCard", map[string]any{"id": matchID, "revision": next})
		require.Contains(t, []any{"MatchNotYourTurnError", "RevisionConflictError", "MatchPhaseNotAllowedError"}, result["__typename"])
	}

	final := waitMatch(t, p.token, matchID, func(m map[string]any) bool { return m["phase"] == "ENDED" })
	outcome := final["outcome"].(map[string]any)
	require.Equal(t, "ROUND_LIMIT", outcome["reason"])
	require.Equal(t, float64(2), final["round"])
	// 勝つのは魔力が最も多い人（同じならすべて）
	var best float64 = -1
	var expected []any
	for i, participant := range final["participants"].([]any) {
		magic := participant.(map[string]any)["magic"].(float64)
		switch {
		case magic > best:
			best, expected = magic, []any{float64(i)}
		case magic == best:
			expected = append(expected, float64(i))
		}
	}
	require.Equal(t, expected, outcome["winners"])
	// 終わった対戦への操作は、段階が合わないものとして断られる
	ended := matchOperation(t, p.token, "endMatchTurn", map[string]any{"id": matchID, "revision": final["revision"]})
	require.Equal(t, "MatchPhaseNotAllowedError", ended["__typename"])
}

// プレイヤーの番に、今の段階に合わない操作・範囲の外の入力・古い revision・参加していない人の操作が断られ、何も変わらないこと
func assertOperationsAreRejected(t *testing.T, token, strangerToken, matchID string, m map[string]any) {
	t.Helper()
	revision := m["revision"]
	input := map[string]any{"id": matchID, "revision": revision}
	// 段階が違う操作（引く段階の中では、振れない）
	if m["phase"] != "ROLLING" {
		require.Equal(t, "MatchPhaseNotAllowedError", matchOperation(t, token, "rollMatchDice", input)["__typename"])
	}
	stale := matchOperation(t, token, "endMatchTurn", map[string]any{"id": matchID, "revision": int(revision.(float64)) - 1})
	require.Equal(t, "RevisionConflictError", stale["__typename"])
	hidden := matchOperation(t, strangerToken, "drawMatchCard", input)
	require.Equal(t, "MatchNotFoundError", hidden["__typename"])
	invalid := matchOperation(t, token, "discardMatchCard", map[string]any{"id": matchID, "revision": revision, "handIndex": 99})
	if m["phase"] == "DISCARDING" {
		require.Equal(t, "ValidationErrors", invalid["__typename"])
		require.Equal(t, []any{map[string]any{"__typename": "FieldOutOfRange", "field": "handIndex", "min": float64(0), "max": float64(len(m["participants"].([]any)[0].(map[string]any)["hand"].([]any)) - 1)}}, invalid["errors"])
	} else {
		require.Equal(t, "MatchPhaseNotAllowedError", invalid["__typename"])
	}
	// (9,9) は盤面に無く、選べる行き先の中に無い
	unselectable := matchOperation(t, token, "chooseMatchPath", map[string]any{"id": matchID, "revision": revision, "to": map[string]any{"x": 9, "y": 9}})
	if m["phase"] == "CHOOSING_PATH" {
		require.Equal(t, "ValidationErrors", unselectable["__typename"])
		require.Equal(t, []any{map[string]any{"__typename": "FieldNotFound", "field": "to"}}, unselectable["errors"])
	} else {
		require.Equal(t, "MatchPhaseNotAllowedError", unselectable["__typename"])
	}
}
