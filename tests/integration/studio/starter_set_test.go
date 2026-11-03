package studio_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

const starterSetFields = `id name status revision registeredAt updatedAt grantDecks { id name status } unlockBoards { id name status }`

const starterSetQuery = `query ($id: UUID!) {
	starterSet(id: $id) {
		__typename
		... on StarterSet { ` + starterSetFields + ` }
		... on StarterSetNotFoundError { message }
	}
}`

const starterSetsListQuery = `query ($filter: StarterSetFilter, $first: Int, $after: String) {
	starterSets(filter: $filter, first: $first, after: $after) {
		__typename
		... on StarterSetConnection {
			edges { cursor node { ` + starterSetFields + ` } }
			pageInfo { hasNextPage }
		}
		... on ValidationErrors { errors { __typename field } }
	}
}`

const starterSetErrorFields = `... on ValidationErrors { errors { __typename field ... on FieldTooLong { max } } }`

const registerStarterSetMutation = `mutation ($input: RegisterStarterSetInput!) {
	registerStarterSet(input: $input) {
		__typename
		... on RegisterStarterSetSuccess { id revision }
		` + starterSetErrorFields + `
	}
}`

const renameStarterSetMutation = `mutation ($input: RenameStarterSetInput!) {
	renameStarterSet(input: $input) {
		__typename
		... on RenameStarterSetSuccess { id revision }
		` + starterSetErrorFields + `
	}
}`

const setStarterSetGrantDecksMutation = `mutation ($input: SetStarterSetGrantDecksInput!) {
	setStarterSetGrantDecks(input: $input) {
		__typename
		... on SetStarterSetGrantDecksSuccess { id revision }
		` + starterSetErrorFields + `
	}
}`

const setStarterSetUnlockBoardsMutation = `mutation ($input: SetStarterSetUnlockBoardsInput!) {
	setStarterSetUnlockBoards(input: $input) {
		__typename
		... on SetStarterSetUnlockBoardsSuccess { id revision }
		` + starterSetErrorFields + `
	}
}`

const deleteStarterSetMutation = `mutation ($input: DeleteStarterSetInput!) {
	deleteStarterSet(input: $input) {
		__typename
		... on DeleteStarterSetSuccess { id }
	}
}`

const publishStarterSetMutation = `mutation ($input: PublishStarterSetInput!) {
	publishStarterSet(input: $input) {
		__typename
		... on PublishStarterSetSuccess { id revision }
		... on StarterSetLimitReachedError { max }
		` + starterSetErrorFields + `
	}
}`

const retireStarterSetMutation = `mutation ($input: RetireStarterSetInput!) {
	retireStarterSet(input: $input) {
		__typename
		... on RetireStarterSetSuccess { id revision }
	}
}`

// 名前が 20 文字までで、UUID（36 文字）をそのまま付けられないため、先頭の 8 文字を使う
func starterSetUniqueID(t *testing.T) string {
	t.Helper()
	return testhelper.UniqueID(t)[:8]
}

func starterSetOperation(t *testing.T, token, mutation, field string, input map[string]any) map[string]any {
	t.Helper()
	return client.Data(t, token, mutation, map[string]any{"input": input}, field)
}

func registerStarterSet(t *testing.T, token, name string) map[string]any {
	t.Helper()
	result := starterSetOperation(t, token, registerStarterSetMutation, "registerStarterSet", map[string]any{"name": name})
	require.Equal(t, "RegisterStarterSetSuccess", result["__typename"])
	return result
}

func starterSetInput(starterSet map[string]any, extra map[string]any) map[string]any {
	input := map[string]any{"id": starterSet["id"], "revision": starterSet["revision"]}
	for k, v := range extra {
		input[k] = v
	}
	return input
}

func setStarterSetGrantDecks(t *testing.T, token string, starterSet map[string]any, grantDecks ...any) map[string]any {
	t.Helper()
	// grantDeckIds は必須（空でも [] を送る）
	return starterSetOperation(t, token, setStarterSetGrantDecksMutation, "setStarterSetGrantDecks", starterSetInput(starterSet, map[string]any{"grantDeckIds": append([]any{}, grantDecks...)}))
}

func setStarterSetUnlockBoards(t *testing.T, token string, starterSet map[string]any, boards ...any) map[string]any {
	t.Helper()
	return starterSetOperation(t, token, setStarterSetUnlockBoardsMutation, "setStarterSetUnlockBoards", starterSetInput(starterSet, map[string]any{"unlockBoardIds": append([]any{}, boards...)}))
}

func publishStarterSet(t *testing.T, token string, starterSet map[string]any) map[string]any {
	t.Helper()
	return starterSetOperation(t, token, publishStarterSetMutation, "publishStarterSet", starterSetInput(starterSet, nil))
}

func retireStarterSet(t *testing.T, token string, starterSet map[string]any) map[string]any {
	t.Helper()
	return starterSetOperation(t, token, retireStarterSetMutation, "retireStarterSet", starterSetInput(starterSet, nil))
}

// 公開中のスターターセットはゲーム全体で 1 つだけなので、公開するテストは並列にしない（並列のテストは、並列でないテストが
// すべて終わってから動く）。終わったら使用不可に戻し、次のテストが公開できるようにする
func retireStarterSetOnCleanup(t *testing.T, token string, starterSet map[string]any) {
	t.Helper()
	t.Cleanup(func() {
		// Cleanup が動くときには t.Context() が取り消されているため、取り消しを引き継がない context を使う
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
		defer cancel()
		latest, err := client.TryData(ctx, token, starterSetQuery, map[string]any{"id": starterSet["id"]}, "starterSet")
		if err == nil && latest["status"] == "PUBLISHED" {
			_, _ = client.TryData(ctx, token, retireStarterSetMutation, map[string]any{"input": map[string]any{"id": latest["id"], "revision": latest["revision"]}}, "retireStarterSet")
		}
	})
}

// 公開中の配布デッキ（40 枚）を作り、投影されるまで待つ
func publishedGrantDeck(t *testing.T, token, name string) map[string]any {
	t.Helper()
	deck := fullGrantDeck(t, token, name)
	published := publishGrantDeck(t, token, deck)
	require.Equal(t, "PublishGrantDeckSuccess", published["__typename"])
	return waitGrantDeck(t, token, published["id"], 3)
}

// 公開中のボードを作り、投影されるまで待つ
func publishedBoard(t *testing.T, token, name string) map[string]any {
	t.Helper()
	changed := changePublishableLayout(t, token, registerBoard(t, token, name))
	published := boardOperation(t, token, publishBoardMutation, "publishBoard", boardInput(changed, nil))
	require.Equal(t, "PublishBoardSuccess", published["__typename"])
	return waitBoard(t, token, published["id"], 3)
}

// 公開中の配布デッキを 1 つ指定した、準備中のスターターセット（変更の結果。id と revision を持つ）
func readyStarterSet(t *testing.T, token, name string) map[string]any {
	t.Helper()
	starterSet := registerStarterSet(t, token, name)
	deck := publishedGrantDeck(t, token, "デッキ-"+name)
	assigned := setStarterSetGrantDecks(t, token, starterSet, deck["id"])
	require.Equal(t, "SetStarterSetGrantDecksSuccess", assigned["__typename"])
	return assigned
}

// 投影されるまで待ち、revision が指定のものになった詳細を返す
func waitStarterSet(t *testing.T, token string, id any, revision int) map[string]any {
	t.Helper()
	var starterSet map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		starterSet, err = client.TryData(t.Context(), token, starterSetQuery, map[string]any{"id": id}, "starterSet")
		if assert.NoError(c, err) {
			assert.Equal(c, "StarterSet", starterSet["__typename"])
			assert.EqualValues(c, revision, starterSet["revision"])
		}
	})
	return starterSet
}

func TestRegisterStarterSet_ListsDraftStarterSet(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := starterSetUniqueID(t)

	registered := registerStarterSet(t, token, " 標準-"+id+" ")

	require.EqualValues(t, 1, registered["revision"])
	var connection map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		connection, err = client.TryData(t.Context(), token, starterSetsListQuery, map[string]any{"filter": map[string]any{"nameContains": id}}, "starterSets")
		if assert.NoError(c, err) {
			assert.Len(c, connection["edges"], 1)
		}
	})
	node := connection["edges"].([]any)[0].(map[string]any)["node"].(map[string]any)
	require.Equal(t, registered["id"], node["id"])
	require.Equal(t, "標準-"+id, node["name"])
	require.Equal(t, "DRAFT", node["status"])
	require.Equal(t, node["registeredAt"], node["updatedAt"])
	require.Equal(t, []any{}, node["grantDecks"])
	require.Equal(t, []any{}, node["unlockBoards"])
	detail := waitStarterSet(t, token, registered["id"], 1)
	delete(detail, "__typename")
	require.Equal(t, node, detail)
}

func TestRegisterStarterSet_ReturnsFieldErrorForTooLongName(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)

	result := starterSetOperation(t, token, registerStarterSetMutation, "registerStarterSet", map[string]any{"name": strings.Repeat("あ", 21)})

	require.Equal(t, "ValidationErrors", result["__typename"])
	require.Equal(t, []any{map[string]any{"__typename": "FieldTooLong", "field": "name", "max": float64(20)}}, result["errors"])
}

func TestStarterSets_RejectsEmptyStatusesAndReturnsNotFoundForUnknownID(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)

	empty := client.Data(t, token, starterSetsListQuery, map[string]any{"filter": map[string]any{"statuses": []string{}}}, "starterSets")
	unknown := client.Data(t, token, starterSetQuery, map[string]any{"id": uuid.NewString()}, "starterSet")

	require.Equal(t, "ValidationErrors", empty["__typename"])
	require.Equal(t, "StarterSetNotFoundError", unknown["__typename"])
}

func TestRenameStarterSet_ShowsNewNameAndFreesOldName(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := starterSetUniqueID(t)
	starterSet := registerStarterSet(t, token, "標準-"+id)

	renamed := starterSetOperation(t, token, renameStarterSetMutation, "renameStarterSet", starterSetInput(starterSet, map[string]any{"name": "新標準-" + id}))

	require.Equal(t, "RenameStarterSetSuccess", renamed["__typename"])
	require.EqualValues(t, 2, renamed["revision"])
	require.Equal(t, "新標準-"+id, waitStarterSet(t, token, starterSet["id"], 2)["name"])
	registerStarterSet(t, token, "標準-"+id)
}

func TestSetStarterSetGrantDecksAndUnlockBoards_ShowThemInGivenOrderAndReplaceThem(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := starterSetUniqueID(t)
	starterSet := registerStarterSet(t, token, "標準-"+id)
	firstDeck := publishedGrantDeck(t, token, "一つ目-"+id)
	secondDeck := publishedGrantDeck(t, token, "二つ目-"+id)
	firstBoard := publishedBoard(t, token, "ボード一-"+id)
	secondBoard := publishedBoard(t, token, "ボード二-"+id)

	decks := setStarterSetGrantDecks(t, token, starterSet, secondDeck["id"], firstDeck["id"])
	sameDecks := setStarterSetGrantDecks(t, token, decks, firstDeck["id"], secondDeck["id"])
	boards := setStarterSetUnlockBoards(t, token, decks, secondBoard["id"], firstBoard["id"])

	require.Equal(t, "SetStarterSetGrantDecksSuccess", decks["__typename"])
	require.EqualValues(t, 2, decks["revision"])
	require.Equal(t, decks, sameDecks)
	require.Equal(t, "SetStarterSetUnlockBoardsSuccess", boards["__typename"])
	require.EqualValues(t, 3, boards["revision"])
	detail := waitStarterSet(t, token, starterSet["id"], 3)
	require.Equal(t, []any{
		map[string]any{"id": secondDeck["id"], "name": "二つ目-" + id, "status": "PUBLISHED"},
		map[string]any{"id": firstDeck["id"], "name": "一つ目-" + id, "status": "PUBLISHED"},
	}, detail["grantDecks"])
	require.Equal(t, []any{
		map[string]any{"id": secondBoard["id"], "name": "ボード二-" + id, "status": "PUBLISHED"},
		map[string]any{"id": firstBoard["id"], "name": "ボード一-" + id, "status": "PUBLISHED"},
	}, detail["unlockBoards"])

	replaced := setStarterSetGrantDecks(t, token, boards, firstDeck["id"])

	// 二つ目だけを外す（割り当てる分は無い）ので、revision は 1 つだけ進む
	require.EqualValues(t, 4, replaced["revision"])
	require.Len(t, waitStarterSet(t, token, starterSet["id"], 4)["grantDecks"], 1)
}

func TestSetStarterSetGrantDecksAndUnlockBoards_ReturnFieldErrorPerItem(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := starterSetUniqueID(t)
	starterSet := registerStarterSet(t, token, "標準-"+id)
	publishedDeck := publishedGrantDeck(t, token, "公開-"+id)
	draftDeck := fullGrantDeck(t, token, "準備中-"+id)
	publishedBoardResult := publishedBoard(t, token, "公開ボード-"+id)
	draftBoard := registerBoard(t, token, "準備中ボード-"+id)

	duplicatedDecks := setStarterSetGrantDecks(t, token, starterSet, publishedDeck["id"], publishedDeck["id"])
	unknownDeck := setStarterSetGrantDecks(t, token, starterSet, uuid.NewString())
	notPublishedDeck := setStarterSetGrantDecks(t, token, starterSet, draftDeck["id"], publishedDeck["id"])
	notABoard := setStarterSetUnlockBoards(t, token, starterSet, publishedDeck["id"])
	notPublishedBoard := setStarterSetUnlockBoards(t, token, starterSet, publishedBoardResult["id"], draftBoard["id"])

	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldDuplicated", "field": "grantDeckIds.1"},
	}}, duplicatedDecks)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldNotFound", "field": "grantDeckIds.0"},
	}}, unknownDeck)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldUnavailable", "field": "grantDeckIds.0"},
	}}, notPublishedDeck)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldNotFound", "field": "unlockBoardIds.0"},
	}}, notABoard)
	require.Equal(t, map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldUnavailable", "field": "unlockBoardIds.1"},
	}}, notPublishedBoard)
}

// 配るデッキが無いと公開できない（解放するボードがあっても）。公開の前に確かめるので、公開中の数は使わず、並列でよい
func TestPublishStarterSet_ReturnsFieldErrorWhenNoGrantDeck(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := starterSetUniqueID(t)
	empty := registerStarterSet(t, token, "空-"+id)
	withBoard := registerStarterSet(t, token, "ボードだけ-"+id)
	board := publishedBoard(t, token, "ボード-"+id)
	withBoard = setStarterSetUnlockBoards(t, token, withBoard, board["id"])
	require.Equal(t, "SetStarterSetUnlockBoardsSuccess", withBoard["__typename"])

	emptyResult := publishStarterSet(t, token, empty)
	withBoardResult := publishStarterSet(t, token, withBoard)

	want := map[string]any{"__typename": "ValidationErrors", "errors": []any{
		map[string]any{"__typename": "FieldRequired", "field": "grantDecks"},
	}}
	require.Equal(t, want, emptyResult)
	require.Equal(t, want, withBoardResult)
}

func TestPublishStarterSet_PublishesRetiresAndPublishesAgain(t *testing.T) {
	token := operators.Token(t)
	starterSet := readyStarterSet(t, token, "標準-"+starterSetUniqueID(t))
	retireStarterSetOnCleanup(t, token, starterSet)

	published := publishStarterSet(t, token, starterSet)
	again := publishStarterSet(t, token, published)
	retired := retireStarterSet(t, token, published)
	sameRetired := retireStarterSet(t, token, retired)
	republished := publishStarterSet(t, token, retired)

	require.EqualValues(t, 3, published["revision"])
	require.Equal(t, published["revision"], again["revision"])
	require.EqualValues(t, 4, retired["revision"])
	require.Equal(t, retired, sameRetired)
	require.EqualValues(t, 5, republished["revision"])
	require.Equal(t, "PUBLISHED", waitStarterSet(t, token, starterSet["id"], 5)["status"])
}

// 公開中は同時に 1 つだけ。2 つ目の公開は上限のエラー（上限の値つき）で、今のセットを使用不可にすると公開できる
func TestPublishStarterSet_ReturnsLimitReachedWhileAnotherIsPublished(t *testing.T) {
	token := operators.Token(t)
	id := starterSetUniqueID(t)
	first := readyStarterSet(t, token, "一-"+id)
	second := readyStarterSet(t, token, "二-"+id)
	retireStarterSetOnCleanup(t, token, first)
	retireStarterSetOnCleanup(t, token, second)

	firstPublished := publishStarterSet(t, token, first)
	limitReached := publishStarterSet(t, token, second)
	retired := retireStarterSet(t, token, firstPublished)
	secondPublished := publishStarterSet(t, token, second)
	republishLimit := publishStarterSet(t, token, retired)

	require.Equal(t, "PublishStarterSetSuccess", firstPublished["__typename"])
	require.Equal(t, map[string]any{"__typename": "StarterSetLimitReachedError", "max": float64(1)}, limitReached)
	require.Equal(t, "RetireStarterSetSuccess", retired["__typename"])
	require.Equal(t, "PublishStarterSetSuccess", secondPublished["__typename"])
	require.Equal(t, map[string]any{"__typename": "StarterSetLimitReachedError", "max": float64(1)}, republishLimit)
	require.Equal(t, "PUBLISHED", waitStarterSet(t, token, second["id"], 3)["status"])
}

func TestStarterSetOperations_ReturnErrorTypesForNameStatusAndRevision(t *testing.T) {
	token := operators.Token(t)
	id := starterSetUniqueID(t)
	registerStarterSet(t, token, "標準-"+id)
	starterSet := readyStarterSet(t, token, "新標準-"+id)
	retireStarterSetOnCleanup(t, token, starterSet)

	used := starterSetOperation(t, token, renameStarterSetMutation, "renameStarterSet", starterSetInput(starterSet, map[string]any{"name": " 標準-" + id}))
	retireDraft := retireStarterSet(t, token, starterSet)
	stale := starterSetOperation(t, token, renameStarterSetMutation, "renameStarterSet", map[string]any{"id": starterSet["id"], "revision": 7, "name": "別-" + id})
	published := publishStarterSet(t, token, starterSet)
	renamePublished := starterSetOperation(t, token, renameStarterSetMutation, "renameStarterSet", starterSetInput(published, map[string]any{"name": "別-" + id}))
	deletePublished := starterSetOperation(t, token, deleteStarterSetMutation, "deleteStarterSet", starterSetInput(published, nil))
	changeDecksPublished := setStarterSetGrantDecks(t, token, published)
	changeBoardsPublished := setStarterSetUnlockBoards(t, token, published)

	require.Equal(t, "StarterSetNameAlreadyInUseError", used["__typename"])
	require.Equal(t, "InvalidStarterSetStatusTransitionError", retireDraft["__typename"])
	require.Equal(t, "RevisionConflictError", stale["__typename"])
	require.Equal(t, "StarterSetNotDraftError", renamePublished["__typename"])
	require.Equal(t, "StarterSetNotDraftError", deletePublished["__typename"])
	require.Equal(t, "StarterSetNotDraftError", changeDecksPublished["__typename"])
	require.Equal(t, "StarterSetNotDraftError", changeBoardsPublished["__typename"])
}

func TestDeleteStarterSet_RemovesDraftStarterSetAndFreesName(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	id := starterSetUniqueID(t)
	starterSet := registerStarterSet(t, token, "標準-"+id)
	waitStarterSet(t, token, starterSet["id"], 1)

	deleted := starterSetOperation(t, token, deleteStarterSetMutation, "deleteStarterSet", starterSetInput(starterSet, nil))
	again := starterSetOperation(t, token, deleteStarterSetMutation, "deleteStarterSet", starterSetInput(starterSet, nil))

	require.Equal(t, map[string]any{"__typename": "DeleteStarterSetSuccess", "id": starterSet["id"]}, deleted)
	require.Equal(t, "StarterSetNotFoundError", again["__typename"])
	testhelper.Eventually(t, func(c *assert.CollectT) {
		result, err := client.TryData(t.Context(), token, starterSetQuery, map[string]any{"id": starterSet["id"]}, "starterSet")
		if assert.NoError(c, err) {
			assert.Equal(c, "StarterSetNotFoundError", result["__typename"])
		}
	})
	registerStarterSet(t, token, "標準-"+id)
}
