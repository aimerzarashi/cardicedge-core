package game_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

const roomFields = `id name revision status
	participants { playerId owner ready }
	seats { kind playerId avatar { id name } deck { id name } }
	matchRules { board { id name } targetMagic startingMagic lapBonus checkpointBonus roundLimit diceMin diceMax baseLandValue }`

// 見ている人にできる操作（部屋の詳細だけが返す）
const roomCapabilityFields = `capabilities { changeSeats changeMatchRules publish declareReady close rename }
	seats { choiceChangeable }`

const roomQuery = `query ($id: UUID!) {
	room(id: $id) {
		__typename
		... on Room { ` + roomFields + ` }
		... on RoomNotFoundError { message }
		... on PlayerNotRegisteredError { message }
	}
}`

const myRoomsQuery = `query ($first: Int, $after: String) {
	myRooms(first: $first, after: $after) {
		__typename
		... on RoomsConnection {
			edges { cursor node { id revision status } }
			pageInfo { hasNextPage hasPreviousPage startCursor endCursor }
		}
		... on ValidationErrors { errors { __typename field ... on FieldOutOfRange { min max } } }
		... on PlayerNotRegisteredError { message }
	}
}`

const createRoomMutation = `mutation ($input: CreateRoomInput!) {
	createRoom(input: $input) {
		__typename
		... on CreateRoomSuccess { id revision }
		` + roomNameValidationFragment + `
		... on RoomRequirementsNotMetError { missing }
		... on PlayerNotRegisteredError { message }
	}
}`

const roomNameValidationFragment = `... on ValidationErrors { errors { __typename field ... on FieldTooLong { max } } }`

const roomCapabilitiesQuery = `query ($id: UUID!) {
	room(id: $id) {
		__typename
		... on Room { revision ` + roomCapabilityFields + ` }
	}
}`

// 変える操作の、入力の型と、結果の union が持つエラーの型（union に入っていない型の断片は書けない）
var roomOperations = map[string]struct {
	input  string
	errors []string
}{
	"changeRoomSeats": {"ChangeRoomSeatsInput", []string{"RoomNotFoundError", "RevisionConflictError", "RoomStatusNotAllowedError", "RoomNotResponsibleError"}},
	"changeRoomMatchRules": {"ChangeRoomMatchRulesInput", []string{
		"RoomNotFoundError", "RevisionConflictError", "RoomStatusNotAllowedError", "RoomConditionLockedError", "RoomNotResponsibleError", "RoomAlreadyReadyError",
	}},
	"changeRoomSeatChoice": {"ChangeRoomSeatChoiceInput", []string{
		"RoomNotFoundError", "RevisionConflictError", "RoomStatusNotAllowedError", "RoomSeatsNotDecidedError", "RoomConditionLockedError", "RoomNotResponsibleError", "RoomAlreadyReadyError",
	}},
	"publishRoom": {"PublishRoomInput", []string{
		"RoomNotFoundError", "RevisionConflictError", "RoomStatusNotAllowedError", "RoomSeatsNotDecidedError", "RoomNotResponsibleError",
	}},
	"declareRoomReady": {"DeclareRoomReadyInput", []string{
		"RoomNotFoundError", "RevisionConflictError", "RoomStatusNotAllowedError", "RoomConditionsNotDecidedError", "RoomAlreadyReadyError", "RoomNotResponsibleError",
	}},
	"renameRoom": {"RenameRoomInput", []string{"RoomNotFoundError", "RevisionConflictError", "RoomStatusNotAllowedError", "RoomNotResponsibleError"}},
	"closeRoom":  {"CloseRoomInput", []string{"RoomNotFoundError", "RevisionConflictError", "RoomStatusNotAllowedError", "RoomNotResponsibleError"}},
}

const roomValidationFragment = `... on ValidationErrors {
	errors {
		__typename field
		... on FieldOutOfRange { min max }
		... on FieldCountMismatch { expected }
		... on FieldTooSmall { min }
		... on FieldNotGreaterThan { than }
	}
}`

func roomOperationQuery(name string) string {
	operation := roomOperations[name]
	success := strings.ToUpper(name[:1]) + name[1:] + "Success"
	var fragments strings.Builder
	for _, errorType := range operation.errors {
		fmt.Fprintf(&fragments, "... on %s { message }\n", errorType)
	}
	validation := ""
	if name == "renameRoom" {
		validation = roomNameValidationFragment
	}
	if name == "changeRoomSeats" || name == "changeRoomMatchRules" || name == "changeRoomSeatChoice" {
		validation = roomValidationFragment
	}
	return fmt.Sprintf(`mutation ($input: %s!) {
		%s(input: $input) {
			__typename
			... on %s { id revision }
			%s
			%s
			... on PlayerNotRegisteredError { message }
		}
	}`, operation.input, name, success, validation, fragments.String())
}

func roomOperation(t *testing.T, token, name string, input map[string]any) map[string]any {
	t.Helper()
	return client.Data(t, token, roomOperationQuery(name), map[string]any{"input": input}, name)
}

// 成功を確かめ、新しい revision を返す
func roomRevision(t *testing.T, result map[string]any, name string) int {
	t.Helper()
	require.Equal(t, strings.ToUpper(name[:1])+name[1:]+"Success", result["__typename"])
	return int(result["revision"].(float64))
}

const roomRequirementsQuery = `query { me { __typename ... on Player { roomRequirements } } }`

// 部屋を作れる（アバターとデッキを持つ）プレイヤーのトークン。デッキはスターターの配布で届くので、足りないものが無くなるまで待つ
func roomOwnerToken(t *testing.T) string {
	t.Helper()
	token := registeredPlayerToken(t)
	registerAvatar(t, token, "ルーム")
	waitRoomRequirements(t, token, []any{})
	return token
}

func waitRoomRequirements(t *testing.T, token string, want []any) {
	t.Helper()
	testhelper.Eventually(t, func(c *assert.CollectT) {
		result, err := client.TryData(t.Context(), token, roomRequirementsQuery, nil, "me")
		if assert.NoError(c, err) {
			assert.Equal(c, want, result["roomRequirements"])
		}
	})
}

func createRoom(t *testing.T, token string) (id string, revision int) {
	t.Helper()
	return createNamedRoom(t, token, "ルーム")
}

func createNamedRoom(t *testing.T, token, name string) (id string, revision int) {
	t.Helper()
	result := client.Data(t, token, createRoomMutation, map[string]any{"input": map[string]any{"name": name}}, "createRoom")
	require.Equal(t, "CreateRoomSuccess", result["__typename"])
	return result["id"].(string), int(result["revision"].(float64))
}

func getRoom(t *testing.T, token, id string) map[string]any {
	t.Helper()
	return client.Data(t, token, roomQuery, map[string]any{"id": id}, "room")
}

// 投影が遅れて反映されるため、画面が見る revision になるまで待つ
func waitRoom(t *testing.T, token, id string, revision int) map[string]any {
	t.Helper()
	var room map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		result, err := client.TryData(t.Context(), token, roomQuery, map[string]any{"id": id}, "room")
		if !assert.NoError(c, err) || !assert.Equal(c, "Room", result["__typename"]) {
			return
		}
		assert.Equal(c, float64(revision), result["revision"])
		room = result
	})
	return room
}

// 部屋で選ぶものがそろったプレイヤー: アバター 1 つと、スターターの配布で届いたデッキ・ボード・NPC
type roomPlayer struct {
	token      string
	avatar     map[string]any
	deck       map[string]any
	playerID   string
	otherDeck  string
	board, npc map[string]any
}

func newRoomPlayer(t *testing.T) roomPlayer {
	t.Helper()
	token := newPlayerToken(t)
	playerID := registeredPlayerID(t, registerPlayer(t, token))
	avatar := registeredAvatar(t, registerAvatar(t, token, "ルーム"))
	player := roomPlayer{token: token, avatar: avatar, playerID: playerID}
	// 配布の流れと投影が済み、画面の選択肢に出るまで待つ
	testhelper.Eventually(t, func(c *assert.CollectT) {
		decks, err := meDecks(t, token)
		if !assert.NoError(c, err) || !assert.Len(c, decks, 2) {
			return
		}
		boards, err := meBoards(t, token)
		if !assert.NoError(c, err) || !assert.Len(c, boards, 2) {
			return
		}
		npcs, err := meNpcAvatars(t, token)
		if !assert.NoError(c, err) || !assert.Len(c, npcs, 2) {
			return
		}
		player.deck = map[string]any{"id": decks[0]["id"], "name": decks[0]["name"]}
		player.otherDeck = decks[1]["id"].(string)
		player.board = map[string]any{"id": boards[1].(map[string]any)["id"], "name": boards[1].(map[string]any)["name"]}
		player.npc = npcs[1].(map[string]any)
	})
	waitMeAvatars(t, token, []map[string]any{avatar})
	return player
}

func (p roomPlayer) rulesInput(id string, revision int, boardID any) map[string]any {
	return map[string]any{
		"id": id, "revision": revision, "boardId": boardID,
		"targetMagic": 5000, "startingMagic": 300, "lapBonus": 200, "checkpointBonus": 50, "roundLimit": 20, "diceMin": 1, "diceMax": 8, "baseLandValue": 100,
	}
}

func rulesResult(board map[string]any) map[string]any {
	return map[string]any{
		"board": board, "targetMagic": float64(5000), "startingMagic": float64(300), "lapBonus": float64(200), "checkpointBonus": float64(50),
		"roundLimit": float64(20), "diceMin": float64(1), "diceMax": float64(8), "baseLandValue": float64(100),
	}
}

func idAndName(value map[string]any) map[string]any {
	return map[string]any{"id": value["id"], "name": value["name"]}
}

// 作る → 席 → 対戦ルール → 席の選択（自分のアバターとデッキ、NPC のアバターとそのデッキ）→ 公開 → 準備中 → 準備完了
func TestRoom_CreatesDecidesConditionsPublishesAndDeclaresReady(t *testing.T) {
	t.Parallel()
	p := newRoomPlayer(t)

	id, revision := createRoom(t, p.token)
	created := waitRoom(t, p.token, id, revision)
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeats", map[string]any{"id": id, "revision": revision, "seats": []string{"PLAYER", "NPC"}}), "changeRoomSeats")
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomMatchRules", p.rulesInput(id, revision, p.board["id"])), "changeRoomMatchRules")
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeatChoice", map[string]any{
		"id": id, "revision": revision, "seat": 0, "avatarId": p.avatar["id"], "deckId": p.deck["id"],
	}), "changeRoomSeatChoice")
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeatChoice", map[string]any{
		"id": id, "revision": revision, "seat": 1, "avatarId": p.npc["id"], "deckId": p.npc["decks"].([]any)[0].(map[string]any)["id"],
	}), "changeRoomSeatChoice")
	decided := waitRoom(t, p.token, id, revision)
	published := roomRevision(t, roomOperation(t, p.token, "publishRoom", map[string]any{"id": id, "revision": revision}), "publishRoom")
	preparing := waitRoom(t, p.token, id, published)
	ready := roomRevision(t, roomOperation(t, p.token, "declareRoomReady", map[string]any{"id": id, "revision": published}), "declareRoomReady")
	afterReady := waitRoom(t, p.token, id, ready)

	require.Equal(t, map[string]any{
		"__typename": "Room", "id": id, "name": "ルーム", "revision": float64(1), "status": "DRAFT",
		"participants": []any{map[string]any{"playerId": p.playerID, "owner": true, "ready": false}},
		"seats":        []any{}, "matchRules": nil,
	}, created)
	npcDeck := p.npc["decks"].([]any)[0].(map[string]any)
	require.Equal(t, []any{
		map[string]any{"kind": "PLAYER", "playerId": p.playerID, "avatar": idAndName(p.avatar), "deck": idAndName(p.deck)},
		map[string]any{"kind": "NPC", "playerId": nil, "avatar": idAndName(p.npc), "deck": idAndName(npcDeck)},
	}, decided["seats"])
	require.Equal(t, rulesResult(p.board), decided["matchRules"])
	require.Equal(t, "PREPARING", preparing["status"])
	require.Equal(t, published, revision+2)
	require.Equal(t, []any{map[string]any{"playerId": p.playerID, "owner": true, "ready": false}}, preparing["participants"])
	// 参加するプレイヤーが作った人だけなので、最後の準備完了で対戦が始まり、部屋は対戦中になる
	require.Equal(t, "PLAYING", afterReady["status"])
	require.Equal(t, published+2, ready)
	require.Equal(t, []any{map[string]any{"playerId": p.playerID, "owner": true, "ready": true}}, afterReady["participants"])
}

// 公開前に決めなかったものを準備中に決める。公開前に決めたものは準備中に変えられず、準備完了の後は選び直せない
func TestRoom_DecidesWhatWasNotDecidedBeforePublishInPreparingAndLocksAfterReady(t *testing.T) {
	t.Parallel()
	p := newRoomPlayer(t)
	id, revision := createRoom(t, p.token)
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeats", map[string]any{"id": id, "revision": revision, "seats": []string{"PLAYER", "NPC"}}), "changeRoomSeats")
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeatChoice", map[string]any{
		"id": id, "revision": revision, "seat": 0, "avatarId": p.avatar["id"], "deckId": p.deck["id"],
	}), "changeRoomSeatChoice")
	revision = roomRevision(t, roomOperation(t, p.token, "publishRoom", map[string]any{"id": id, "revision": revision}), "publishRoom")
	npcChoice := func(revision int) map[string]any {
		return map[string]any{"id": id, "revision": revision, "seat": 1, "avatarId": p.npc["id"], "deckId": p.npc["decks"].([]any)[0].(map[string]any)["id"]}
	}

	notDecided := roomOperation(t, p.token, "declareRoomReady", map[string]any{"id": id, "revision": revision})
	lockedChoice := roomOperation(t, p.token, "changeRoomSeatChoice", map[string]any{
		"id": id, "revision": revision, "seat": 0, "avatarId": p.avatar["id"], "deckId": p.otherDeck,
	})
	lockedSeats := roomOperation(t, p.token, "changeRoomSeats", map[string]any{"id": id, "revision": revision, "seats": []string{"PLAYER", "NPC", "NPC"}})
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeatChoice", npcChoice(revision)), "changeRoomSeatChoice")
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomMatchRules", p.rulesInput(id, revision, p.board["id"])), "changeRoomMatchRules")
	// 準備完了までは選び直せる
	rechosen := p.rulesInput(id, revision, p.board["id"])
	rechosen["targetMagic"] = 6000
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomMatchRules", rechosen), "changeRoomMatchRules")
	revision = roomRevision(t, roomOperation(t, p.token, "declareRoomReady", map[string]any{"id": id, "revision": revision}), "declareRoomReady")
	afterReadyChoice := roomOperation(t, p.token, "changeRoomSeatChoice", npcChoice(revision))
	afterReadyRules := roomOperation(t, p.token, "changeRoomMatchRules", p.rulesInput(id, revision, p.board["id"]))
	readyAgain := roomOperation(t, p.token, "declareRoomReady", map[string]any{"id": id, "revision": revision})

	require.Equal(t, "RoomConditionsNotDecidedError", notDecided["__typename"])
	require.Equal(t, "RoomConditionLockedError", lockedChoice["__typename"])
	require.Equal(t, "RoomStatusNotAllowedError", lockedSeats["__typename"])
	// 参加するのが作った人だけなので、準備完了の申告で対戦が始まり、部屋は対戦中になる（対戦中は条件を変えられない）
	require.Equal(t, "RoomStatusNotAllowedError", afterReadyChoice["__typename"])
	require.Equal(t, "RoomStatusNotAllowedError", afterReadyRules["__typename"])
	require.Equal(t, "RoomStatusNotAllowedError", readyAgain["__typename"])
	room := waitRoom(t, p.token, id, revision)
	require.Equal(t, float64(6000), room["matchRules"].(map[string]any)["targetMagic"])
}

func TestRoom_CreateReturnsValidationErrorsForNameAndAllowsSameName(t *testing.T) {
	t.Parallel()
	token := roomOwnerToken(t)
	create := func(name string) map[string]any {
		return client.Data(t, token, createRoomMutation, map[string]any{"input": map[string]any{"name": name}}, "createRoom")
	}

	blank := create("  ")
	long := create(strings.Repeat("あ", 21))
	first, _ := createNamedRoom(t, token, "  同じ名前 ")
	second, _ := createNamedRoom(t, token, "同じ名前")

	require.Equal(t, "ValidationErrors", blank["__typename"])
	require.Equal(t, []any{map[string]any{"__typename": "FieldRequired", "field": "name"}}, blank["errors"])
	require.Equal(t, []any{map[string]any{"__typename": "FieldTooLong", "field": "name", "max": float64(20)}}, long["errors"])
	require.NotEqual(t, first, second)
	require.Equal(t, "同じ名前", waitRoom(t, token, first, 1)["name"])
}

// 名前は閉じる前まで、作った人が変えられる（公開の後にも）。同じ名前は何も変えず、閉じた後は変えられない。一覧と詳細に出る
func TestRoom_RenameChangesNameUntilClosedAndShowsItInListAndDetail(t *testing.T) {
	t.Parallel()
	token := roomOwnerToken(t)
	id, revision := createNamedRoom(t, token, "最初の名前")
	revision = roomRevision(t, roomOperation(t, token, "changeRoomSeats", map[string]any{"id": id, "revision": revision, "seats": []string{"PLAYER", "NPC"}}), "changeRoomSeats")
	revision = roomRevision(t, roomOperation(t, token, "publishRoom", map[string]any{"id": id, "revision": revision}), "publishRoom")

	renamed := roomRevision(t, roomOperation(t, token, "renameRoom", map[string]any{"id": id, "revision": revision, "name": " 新しい名前 "}), "renameRoom")
	same := roomRevision(t, roomOperation(t, token, "renameRoom", map[string]any{"id": id, "revision": renamed, "name": "新しい名前"}), "renameRoom")
	blank := roomOperation(t, token, "renameRoom", map[string]any{"id": id, "revision": renamed, "name": ""})
	stale := roomOperation(t, token, "renameRoom", map[string]any{"id": id, "revision": revision, "name": "古い版"})
	closed := roomRevision(t, roomOperation(t, token, "closeRoom", map[string]any{"id": id, "revision": renamed}), "closeRoom")
	afterClose := roomOperation(t, token, "renameRoom", map[string]any{"id": id, "revision": closed, "name": "閉じた後"})

	require.Equal(t, revision+1, renamed)
	require.Equal(t, renamed, same)
	require.Equal(t, []any{map[string]any{"__typename": "FieldRequired", "field": "name"}}, blank["errors"])
	require.Equal(t, "RevisionConflictError", stale["__typename"])
	require.Equal(t, "RoomStatusNotAllowedError", afterClose["__typename"])
	require.Equal(t, "新しい名前", waitRoom(t, token, id, closed)["name"])
	listedID, _ := createNamedRoom(t, token, "一覧の名前")
	testhelper.Eventually(t, func(c *assert.CollectT) {
		result, err := client.TryData(t.Context(), token, `query { myRooms { __typename ... on RoomsConnection { edges { node { id name } } } } }`, nil, "myRooms")
		if !assert.NoError(c, err) {
			return
		}
		edges, ok := result["edges"].([]any)
		if !assert.True(c, ok) || !assert.NotEmpty(c, edges) {
			return
		}
		assert.Equal(c, map[string]any{"id": listedID, "name": "一覧の名前"}, edges[0].(map[string]any)["node"])
	})
}

// 画面は担当・公開前に決めたものの固定・準備完了の条件を自分で判断せず、room(id) の capabilities と seats.choiceChangeable で出し分ける。
// 作った人にできる操作が、公開前 → 準備中（公開前に決めたものは固定）→ 準備完了 → 閉じた で core の決まり通りに変わる
func TestRoom_CapabilitiesFollowWhatTheOwnerCanDoThroughTheLifecycle(t *testing.T) {
	t.Parallel()
	p := newRoomPlayer(t)
	id, revision := createRoom(t, p.token)
	// 投影が遅れても、できる操作は集約から読むので、画面が見る revision に追いついてから確かめる
	capabilities := func(revision int, seats ...bool) map[string]any {
		var found map[string]any
		testhelper.Eventually(t, func(c *assert.CollectT) {
			result, err := client.TryData(t.Context(), p.token, roomCapabilitiesQuery, map[string]any{"id": id}, "room")
			if !assert.NoError(c, err) || !assert.Equal(c, float64(revision), result["revision"]) {
				return
			}
			found = result["capabilities"].(map[string]any)
			changeable := []bool{}
			for _, seat := range result["seats"].([]any) {
				changeable = append(changeable, seat.(map[string]any)["choiceChangeable"].(bool))
			}
			assert.Equal(c, append([]bool{}, seats...), changeable)
		})
		return found
	}
	want := func(changeSeats, changeMatchRules, publish, declareReady, closable, rename bool) map[string]any {
		return map[string]any{
			"changeSeats": changeSeats, "changeMatchRules": changeMatchRules, "publish": publish,
			"declareReady": declareReady, "close": closable, "rename": rename,
		}
	}

	// 席が無い間は、公開できない
	require.Equal(t, want(true, true, false, false, true, true), capabilities(revision))
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeats", map[string]any{"id": id, "revision": revision, "seats": []string{"PLAYER", "NPC"}}), "changeRoomSeats")
	require.Equal(t, want(true, true, true, false, true, true), capabilities(revision, true, true))
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeatChoice", map[string]any{
		"id": id, "revision": revision, "seat": 0, "avatarId": p.avatar["id"], "deckId": p.deck["id"],
	}), "changeRoomSeatChoice")
	revision = roomRevision(t, roomOperation(t, p.token, "publishRoom", map[string]any{"id": id, "revision": revision}), "publishRoom")
	// 公開前に決めた席は固定され、決めなかった席と対戦ルールは準備中に決められる。担当の分が決まるまで、準備完了は押せない
	require.Equal(t, want(false, true, false, false, true, true), capabilities(revision, false, true))
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeatChoice", map[string]any{
		"id": id, "revision": revision, "seat": 1, "avatarId": p.npc["id"], "deckId": p.npc["decks"].([]any)[0].(map[string]any)["id"],
	}), "changeRoomSeatChoice")
	require.Equal(t, want(false, true, false, false, true, true), capabilities(revision, false, true))
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomMatchRules", p.rulesInput(id, revision, p.board["id"])), "changeRoomMatchRules")
	require.Equal(t, want(false, true, false, true, true, true), capabilities(revision, false, true))
	revision = roomRevision(t, roomOperation(t, p.token, "declareRoomReady", map[string]any{"id": id, "revision": revision}), "declareRoomReady")
	// 準備完了で対戦が始まり（対戦中）、条件は変えられず、閉じられない。名前は変えられる
	require.Equal(t, want(false, false, false, false, false, true), capabilities(revision, false, false))
	closed := roomOperation(t, p.token, "closeRoom", map[string]any{"id": id, "revision": revision})
	require.Equal(t, "RoomStatusNotAllowedError", closed["__typename"])
}

func TestRoom_PublishRequiresSeatsButNotChoicesAndMovesToPreparing(t *testing.T) {
	t.Parallel()
	p := newRoomPlayer(t)
	id, revision := createRoom(t, p.token)

	noSeats := roomOperation(t, p.token, "publishRoom", map[string]any{"id": id, "revision": revision})
	noSeatsChoice := roomOperation(t, p.token, "changeRoomSeatChoice", map[string]any{
		"id": id, "revision": revision, "seat": 0, "avatarId": p.avatar["id"], "deckId": p.deck["id"],
	})
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeats", map[string]any{"id": id, "revision": revision, "seats": []string{"PLAYER", "NPC"}}), "changeRoomSeats")
	published := roomRevision(t, roomOperation(t, p.token, "publishRoom", map[string]any{"id": id, "revision": revision}), "publishRoom")

	require.Equal(t, "RoomSeatsNotDecidedError", noSeats["__typename"])
	require.Equal(t, "RoomSeatsNotDecidedError", noSeatsChoice["__typename"])
	require.Equal(t, revision+2, published)
	require.Equal(t, "PREPARING", waitRoom(t, p.token, id, published)["status"])
}

func TestRoom_ChangeSeatsReturnsValidationErrorsInStages(t *testing.T) {
	t.Parallel()
	p := newRoomPlayer(t)
	id, revision := createRoom(t, p.token)
	change := func(seats ...string) map[string]any {
		return roomOperation(t, p.token, "changeRoomSeats", map[string]any{"id": id, "revision": revision, "seats": seats})
	}

	tooFew := change("PLAYER")
	tooMany := change("PLAYER", "NPC", "NPC", "NPC", "NPC")
	noPlayerSeat := change("NPC", "NPC")
	twoPlayerSeats := change("PLAYER", "PLAYER", "NPC")

	outOfRange := []any{map[string]any{"__typename": "FieldOutOfRange", "field": "seats", "min": float64(2), "max": float64(4)}}
	mismatch := []any{map[string]any{"__typename": "FieldCountMismatch", "field": "seats", "expected": float64(1)}}
	require.Equal(t, "ValidationErrors", tooFew["__typename"])
	require.Equal(t, outOfRange, tooFew["errors"])
	require.Equal(t, outOfRange, tooMany["errors"])
	require.Equal(t, mismatch, noPlayerSeat["errors"])
	require.Equal(t, mismatch, twoPlayerSeats["errors"])
}

func TestRoom_ChangeMatchRulesReturnsValidationErrorsForValuesAndUnselectableBoard(t *testing.T) {
	t.Parallel()
	p := newRoomPlayer(t)
	id, revision := createRoom(t, p.token)
	invalid := p.rulesInput(id, revision, master.Boards.Draft.String())
	invalid["roundLimit"] = 0
	invalid["diceMax"] = 0
	notUnlocked := p.rulesInput(id, revision, uuid.NewString())

	values := roomOperation(t, p.token, "changeRoomMatchRules", invalid)
	board := roomOperation(t, p.token, "changeRoomMatchRules", notUnlocked)

	require.Equal(t, "ValidationErrors", values["__typename"])
	require.ElementsMatch(t, []any{
		map[string]any{"__typename": "FieldTooSmall", "field": "roundLimit", "min": float64(1)},
		map[string]any{"__typename": "FieldNotGreaterThan", "field": "diceMax", "than": "diceMin"},
		map[string]any{"__typename": "FieldNotFound", "field": "boardId"},
	}, values["errors"])
	require.Equal(t, []any{map[string]any{"__typename": "FieldNotFound", "field": "boardId"}}, board["errors"])
}

func TestRoom_ChangeSeatChoiceReturnsValidationErrorsForValuesThatAreNotSelectable(t *testing.T) {
	t.Parallel()
	p := newRoomPlayer(t)
	stranger := newRoomPlayer(t)
	id, revision := createRoom(t, p.token)
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeats", map[string]any{"id": id, "revision": revision, "seats": []string{"PLAYER", "NPC"}}), "changeRoomSeats")
	choose := func(seat int, avatarID, deckID any) []any {
		result := roomOperation(t, p.token, "changeRoomSeatChoice", map[string]any{
			"id": id, "revision": revision, "seat": seat, "avatarId": avatarID, "deckId": deckID,
		})
		require.Equal(t, "ValidationErrors", result["__typename"])
		return result["errors"].([]any)
	}
	notFound := func(field string) map[string]any {
		return map[string]any{"__typename": "FieldNotFound", "field": field}
	}
	npcDeck := p.npc["decks"].([]any)[0].(map[string]any)["id"]

	require.Equal(t, []any{notFound("avatarId"), notFound("deckId")}, choose(0, stranger.avatar["id"], stranger.deck["id"]))
	require.Equal(t, []any{notFound("deckId")}, choose(0, p.avatar["id"], npcDeck))
	require.Equal(t, []any{notFound("avatarId")}, choose(1, p.avatar["id"], npcDeck))
	// 解放していない NPC は、公開中でも選べない。アバターが選べないときは、デッキを確かめない
	require.Equal(t, []any{notFound("avatarId")}, choose(1, master.NPCAvatars.PublishedLocked.String(), master.NPCAvatars.DeckPublished1.String()))
	// 割り当てていない（公開中でも）・割り当てたが準備中のデッキは選べない
	require.Equal(t, []any{notFound("deckId")}, choose(1, p.npc["id"], master.NPCAvatars.DeckUnassigned.String()))
	require.Equal(t, []any{notFound("deckId")}, choose(1, p.npc["id"], p.deck["id"]))
	require.Equal(t, []any{map[string]any{"__typename": "FieldOutOfRange", "field": "seat", "min": float64(0), "max": float64(1)}},
		choose(2, p.avatar["id"], p.deck["id"]))
}

func TestRoom_RejectsStaleRevisionAndRoomsOfOtherPlayers(t *testing.T) {
	t.Parallel()
	p := newRoomPlayer(t)
	stranger := registeredPlayerToken(t)
	id, revision := createRoom(t, p.token)
	revision = roomRevision(t, roomOperation(t, p.token, "changeRoomSeats", map[string]any{"id": id, "revision": revision, "seats": []string{"PLAYER", "NPC"}}), "changeRoomSeats")

	stale := roomOperation(t, p.token, "publishRoom", map[string]any{"id": id, "revision": revision - 1})
	ofOther := roomOperation(t, stranger, "closeRoom", map[string]any{"id": id, "revision": revision})
	unknown := roomOperation(t, p.token, "closeRoom", map[string]any{"id": uuid.NewString(), "revision": 1})

	require.Equal(t, "RevisionConflictError", stale["__typename"])
	require.Equal(t, "RoomNotFoundError", ofOther["__typename"])
	require.Equal(t, "RoomNotFoundError", unknown["__typename"])
	require.Equal(t, "RoomNotFoundError", getRoom(t, stranger, id)["__typename"])
	require.Equal(t, "RoomNotFoundError", getRoom(t, p.token, uuid.NewString())["__typename"])
	// 入っていない人の操作は、部屋を変えない
	require.Equal(t, "DRAFT", waitRoom(t, p.token, id, revision)["status"])
}

func TestRoom_CloseHidesRoomFromMyRoomsButStillReadableAndIsIdempotent(t *testing.T) {
	t.Parallel()
	token := roomOwnerToken(t)
	openID, _ := createRoom(t, token)
	closedID, revision := createRoom(t, token)

	closed := roomRevision(t, roomOperation(t, token, "closeRoom", map[string]any{"id": closedID, "revision": revision}), "closeRoom")
	again := roomRevision(t, roomOperation(t, token, "closeRoom", map[string]any{"id": closedID, "revision": closed}), "closeRoom")
	afterClose := roomOperation(t, token, "publishRoom", map[string]any{"id": closedID, "revision": closed})

	require.Equal(t, closed, again)
	require.Equal(t, "RoomStatusNotAllowedError", afterClose["__typename"])
	require.Equal(t, "CLOSED", waitRoom(t, token, closedID, closed)["status"])
	testhelper.Eventually(t, func(c *assert.CollectT) {
		result, err := client.TryData(t.Context(), token, myRoomsQuery, nil, "myRooms")
		if !assert.NoError(c, err) {
			return
		}
		assert.Equal(c, []string{openID}, roomIDsOf(result))
	})
}

func roomIDsOf(connection map[string]any) []string {
	ids := []string{}
	for _, edge := range connection["edges"].([]any) {
		ids = append(ids, edge.(map[string]any)["node"].(map[string]any)["id"].(string))
	}
	return ids
}

func TestRoom_MyRoomsListsOwnRoomsNewestFirstWithCursorPaging(t *testing.T) {
	t.Parallel()
	token := roomOwnerToken(t)
	other := roomOwnerToken(t)
	first, _ := createRoom(t, token)
	second, _ := createRoom(t, token)
	third, _ := createRoom(t, token)
	createRoom(t, other)

	var firstPage map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		result, err := client.TryData(t.Context(), token, myRoomsQuery, map[string]any{"first": 2}, "myRooms")
		if !assert.NoError(c, err) || !assert.Equal(c, "RoomsConnection", result["__typename"]) {
			return
		}
		assert.Equal(c, []string{third, second}, roomIDsOf(result))
		firstPage = result
	})
	pageInfo := firstPage["pageInfo"].(map[string]any)
	next := client.Data(t, token, myRoomsQuery, map[string]any{"first": 2, "after": pageInfo["endCursor"]}, "myRooms")

	require.Equal(t, true, pageInfo["hasNextPage"])
	require.Equal(t, []string{first}, roomIDsOf(next))
	require.Equal(t, false, next["pageInfo"].(map[string]any)["hasNextPage"])
	require.Equal(t, true, next["pageInfo"].(map[string]any)["hasPreviousPage"])
}

func TestRoom_MyRoomsReturnsValidationErrorsForInvalidPaging(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)

	result := client.Data(t, token, myRoomsQuery, map[string]any{"first": 0, "after": "not-a-cursor"}, "myRooms")

	require.Equal(t, "ValidationErrors", result["__typename"])
	require.ElementsMatch(t, []any{
		map[string]any{"__typename": "FieldOutOfRange", "field": "first", "min": float64(1), "max": float64(100)},
		map[string]any{"__typename": "FieldInvalidFormat", "field": "after"},
	}, result["errors"])
}

func TestRoom_ReturnsPlayerNotRegisteredForUnregisteredPlayer(t *testing.T) {
	t.Parallel()
	token := newPlayerToken(t)

	created := client.Data(t, token, createRoomMutation, map[string]any{"input": map[string]any{"name": "ルーム"}}, "createRoom")
	seats := roomOperation(t, token, "changeRoomSeats", map[string]any{"id": uuid.NewString(), "revision": 1, "seats": []string{"PLAYER", "NPC"}})
	room := getRoom(t, token, uuid.NewString())
	list := client.Data(t, token, myRoomsQuery, nil, "myRooms")

	for _, result := range []map[string]any{created, seats, room, list} {
		require.Equal(t, "PlayerNotRegisteredError", result["__typename"])
		require.NotEmpty(t, result["message"])
	}
}

// アバターかデッキが無いと部屋を作れず、足りないものの一覧が返る。Player.roomRequirements も同じものを返す
func TestRoom_CreateRequiresAnAvatarAndADeckAndRoomRequirementsTellsWhatIsMissing(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)
	// デッキはスターターの配布で届く。届くまで待ってから、アバターだけが無い状態を確かめる
	waitRoomRequirements(t, token, []any{"AVATAR"})

	refused := client.Data(t, token, createRoomMutation, map[string]any{"input": map[string]any{"name": "ルーム"}}, "createRoom")
	registerAvatar(t, token, "ルーム")
	// 数は書く側の記録から読むので、アバターを作った直後から作れる
	created := client.Data(t, token, createRoomMutation, map[string]any{"input": map[string]any{"name": "ルーム"}}, "createRoom")
	requirements := client.Data(t, token, roomRequirementsQuery, nil, "me")

	require.Equal(t, map[string]any{"__typename": "RoomRequirementsNotMetError", "missing": []any{"AVATAR"}}, refused)
	require.Equal(t, "CreateRoomSuccess", created["__typename"])
	require.Equal(t, []any{}, requirements["roomRequirements"])
}
