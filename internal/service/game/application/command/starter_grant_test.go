package command_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/unlock"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
)

// 集約の種別の id（internal/domaindef/catalog.go）
const (
	starterGrantTypeID = 1700
	collectionTypeID   = 1800
	deckTypeID         = 1900
	unlockTypeID       = 2800
)

var (
	cardA = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	cardB = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	cardC = uuid.MustParse("00000000-0000-0000-0000-000000000003")
	// 解放するボードの公開 ID。ボードの中身は読まないので、写した boards は要らない
	boardA = int64(50)
	boardB = int64(51)
	boardC = int64(52)
)

var publishedDecks = []master.GrantDeck{
	{Name: "火", Cards: []master.DeckCard{{CardID: cardA, Count: 4}, {CardID: cardB, Count: 1}}},
	{Name: "水", Cards: []master.DeckCard{{CardID: cardA, Count: 2}, {CardID: cardB, Count: 3}, {CardID: cardC, Count: 2}}},
}

// 公開中のスターターセット。解放するボードの並びはセットが持つ順（公開 ID の順ではない）
// 配るボード。セットが持つ順（ID の順ではない）
var starterBoards = []board.ID{mustBoardID(boardB), mustBoardID(boardA)}

func mustBoardID(value int64) board.ID {
	id, err := board.NewID(value)
	if err != nil {
		panic(err)
	}
	return id
}

func publishedMaster() master.GetPublishedStarterSetFunc {
	return decksMaster(publishedDecks...)
}

func decksMaster(decks ...master.GrantDeck) master.GetPublishedStarterSetFunc {
	return starterSetMaster(master.StarterSet{GrantDecks: decks, UnlockBoards: starterBoards})
}

func starterSetMaster(set master.StarterSet) master.GetPublishedStarterSetFunc {
	return func(context.Context) (master.StarterSet, bool, error) {
		return set, true, nil
	}
}

// 公開中のスターターセットが無い
func unpublishedMaster() master.GetPublishedStarterSetFunc {
	return func(context.Context) (master.StarterSet, bool, error) {
		return master.StarterSet{}, false, nil
	}
}

func TestStart_RejectsWhenMasterNotPublished(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newStarterGrantHandler(t, ctx, unpublishedMaster())

	err := handler.Start(ctx, issuePlayerID(t, ctx))

	require.ErrorIs(t, err, command.ErrMasterNotPublished)
	require.Equal(t, 0, countEventsOf(t, ctx, starterGrantTypeID))
}

// セットはあっても、配る配布デッキが公開中のものが 1 つも無ければ（使用不可にした後など）、配布は始めない
func TestStart_RejectsWhenStarterSetHasNoPublishedGrantDeck(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newStarterGrantHandler(t, ctx, starterSetMaster(master.StarterSet{UnlockBoards: []board.ID{boardtest.ID(t, boardA)}}))

	err := handler.Start(ctx, issuePlayerID(t, ctx))

	require.ErrorIs(t, err, command.ErrMasterNotPublished)
	require.Equal(t, 0, countEventsOf(t, ctx, starterGrantTypeID))
}

func TestStart_CopiesUnlockBoardsInSetOrderFromMaster(t *testing.T) {
	ctx := isolatedDatabase(t)
	playerID := issuePlayerID(t, ctx)

	require.NoError(t, newStarterGrantHandler(t, ctx, publishedMaster()).Start(ctx, playerID))

	state := loadStarterGrant(t, ctx, playerID).(startergrant.InProgressStarterGrant)
	require.Equal(t, []board.ID{boardtest.ID(t, boardB), boardtest.ID(t, boardA)}, state.UnlockBoards)
}

// 解放するボードは 0 件でもよい（配るデッキと違い、無くても配布は始められる）
func TestStart_StartsWhenStarterSetUnlocksNoBoards(t *testing.T) {
	ctx := isolatedDatabase(t)
	playerID := issuePlayerID(t, ctx)
	handler := newStarterGrantHandler(t, ctx, starterSetMaster(master.StarterSet{GrantDecks: publishedDecks, UnlockBoards: []board.ID{}}))

	require.NoError(t, handler.Start(ctx, playerID))

	state := loadStarterGrant(t, ctx, playerID).(startergrant.InProgressStarterGrant)
	require.Empty(t, state.UnlockBoards)
}

func TestStart_CopiesMostCountOfEachCardFromMaster(t *testing.T) {
	ctx := isolatedDatabase(t)
	playerID := issuePlayerID(t, ctx)

	require.NoError(t, newStarterGrantHandler(t, ctx, publishedMaster()).Start(ctx, playerID))

	state, ok := loadStarterGrant(t, ctx, playerID).(startergrant.InProgressStarterGrant)
	require.True(t, ok)
	require.Equal(t, []card.Count{
		cardtest.Count(t, cardA, 4),
		cardtest.Count(t, cardB, 3),
		cardtest.Count(t, cardC, 2),
	}, state.Cards)
	require.Len(t, state.Decks, 2)
}

// セットが指す配布デッキをあるだけ配る（2 つに決めない）。デッキの名前は配布デッキの名前
func TestStart_DistributesAsManyDecksAsStarterSetHas(t *testing.T) {
	tests := []struct {
		name  string
		decks []master.GrantDeck
	}{
		{name: "one_deck", decks: publishedDecks[:1]},
		{name: "three_decks", decks: append(append([]master.GrantDeck{}, publishedDecks...), master.GrantDeck{Name: "風", Cards: []master.DeckCard{{CardID: cardC, Count: 5}}})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := isolatedDatabase(t)
			handler := newStarterGrantHandler(t, ctx, decksMaster(tt.decks...))
			playerID, grantID := startedGrantID(t, ctx, handler)

			require.NoError(t, handler.RecordCollection(ctx, grantID))
			require.NoError(t, handler.Distribute(ctx, grantID))

			require.Equal(t, len(tt.decks), countEventsOf(t, ctx, deckTypeID))
			state := loadStarterGrant(t, ctx, playerID).(startergrant.InProgressStarterGrant)
			require.Len(t, state.Decks, len(tt.decks))
			for i, deck := range tt.decks {
				require.Equal(t, deck.Name, state.Decks[i].Name.String())
			}
		})
	}
}

func TestStart_DoesNothingWhenAlreadyStarted(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newStarterGrantHandler(t, ctx, publishedMaster())
	playerID := issuePlayerID(t, ctx)
	require.NoError(t, handler.Start(ctx, playerID))

	err := handler.Start(ctx, playerID)

	require.NoError(t, err)
	require.Equal(t, 1, countEventsOf(t, ctx, starterGrantTypeID))
}

func startedGrantID(t *testing.T, ctx context.Context, handler *command.StarterGrantHandler) (playerID, grantID int64) {
	t.Helper()
	playerID = issuePlayerID(t, ctx)
	require.NoError(t, handler.Start(ctx, playerID))
	return playerID, loadStarterGrant(t, ctx, playerID).(startergrant.InProgressStarterGrant).ID.Int64()
}

func TestDistribute_CreatesCollectionUnlockAndTwoDecksOnlyOnce(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newStarterGrantHandler(t, ctx, publishedMaster())
	_, grantID := startedGrantID(t, ctx, handler)

	require.NoError(t, handler.Distribute(ctx, grantID))
	require.NoError(t, handler.Distribute(ctx, grantID))

	require.Equal(t, 1, countEventsOf(t, ctx, collectionTypeID))
	require.Equal(t, 1, countEventsOf(t, ctx, unlockTypeID))
	require.Equal(t, 2, countEventsOf(t, ctx, deckTypeID))
}

func TestRecord_CompletesWhenCollectionUnlockAndBothDecksAreRecorded(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newStarterGrantHandler(t, ctx, publishedMaster())
	playerID, grantID := startedGrantID(t, ctx, handler)

	require.NoError(t, handler.RecordCollection(ctx, grantID))
	require.NoError(t, handler.RecordDeck(ctx, grantID, 1))
	require.NoError(t, handler.RecordDeck(ctx, grantID, 2))
	require.IsType(t, startergrant.InProgressStarterGrant{}, loadStarterGrant(t, ctx, playerID))
	require.NoError(t, handler.RecordUnlock(ctx, grantID))

	require.IsType(t, startergrant.CompletedStarterGrant{}, loadStarterGrant(t, ctx, playerID))
}

func TestRecord_DoesNothingWhenRecordedAgain(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newStarterGrantHandler(t, ctx, publishedMaster())
	_, grantID := startedGrantID(t, ctx, handler)
	require.NoError(t, handler.RecordCollection(ctx, grantID))

	err := handler.RecordCollection(ctx, grantID)

	require.NoError(t, err)
	require.Equal(t, 2, countEventsOf(t, ctx, starterGrantTypeID))
}

// 所持カード・解放・デッキ 2 つの記録が同時に来て版がぶつかっても、ポリシーの失敗にせず、読み直して 4 つともそろう
func TestRecord_CompletesWhenRecordedConcurrently(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newStarterGrantHandler(t, ctx, publishedMaster())
	playerID, grantID := startedGrantID(t, ctx, handler)

	var wg sync.WaitGroup
	errs := make([]error, 4)
	wg.Go(func() { errs[0] = handler.RecordCollection(ctx, grantID) })
	wg.Go(func() { errs[1] = handler.RecordDeck(ctx, grantID, 1) })
	wg.Go(func() { errs[2] = handler.RecordDeck(ctx, grantID, 2) })
	wg.Go(func() { errs[3] = handler.RecordUnlock(ctx, grantID) })
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	require.IsType(t, startergrant.CompletedStarterGrant{}, loadStarterGrant(t, ctx, playerID))
}

// 集約のイベントを、コマンドが保存した順に読む（ポリシーが受け取るものと同じ）
func loadEvents[I domain.ID, E domain.Event[I]](t *testing.T, ctx context.Context, newID func(int64) (I, error), newEventType func(domain.EventName) domain.EventType[I, E], id int64) []any {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	loaded, err := eventstore.NewEventLoader(registry, newID, newEventType).LoadEvents(ctx, id)
	require.NoError(t, err)
	events := make([]any, 0, len(loaded))
	for _, e := range loaded {
		events = append(events, e.Event)
	}
	return events
}

func handleAll(t *testing.T, ctx context.Context, handler *command.StarterGrantHandler, events []any) {
	t.Helper()
	for _, event := range events {
		require.NoError(t, handler.Handle(ctx, event))
	}
}

// ポリシーが受け取る順（プレイヤーの登録 → 配布の開始 → 所持カード・解放・デッキの作成）にイベントを渡すと、配布が終わる
func TestHandle_CompletesGrantWhenGivenEventsOfFlow(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newStarterGrantHandler(t, ctx, publishedMaster())
	players := newPlayerHandler(t, ctx)
	_, err := players.Register(authenticate(t, ctx, players, "user-1"))
	require.NoError(t, err)
	playerID := constrainttest.Owner(t, ctx, player.SubjectKey(playertest.Issuer(t, "diecardomain-game"), playertest.Subject(t, "user-1")))

	handleAll(t, ctx, handler, loadEvents(t, ctx, player.NewID, player.NewEventType, playerID))
	grant := loadStarterGrant(t, ctx, playerID).(startergrant.InProgressStarterGrant)
	handleAll(t, ctx, handler, loadEvents(t, ctx, startergrant.NewID, startergrant.NewEventType, grant.ID.Int64()))
	grantPlayer := playertest.ID(t, playerID)
	handleAll(t, ctx, handler, loadEvents(t, ctx, collection.NewID, collection.NewEventType, constrainttest.Owner(t, ctx, collection.PlayerKey(grantPlayer))))
	unlockID := constrainttest.Owner(t, ctx, unlock.PlayerKey(grantPlayer))
	handleAll(t, ctx, handler, loadEvents(t, ctx, unlock.NewID, unlock.NewEventType, unlockID))
	// 作られた解放に、写したボードを解放する（イベントは作成と、ボードの解放の 2 つ）。解放の記録は、ボードの解放のイベントで済む
	unlockEvents := loadEvents(t, ctx, unlock.NewID, unlock.NewEventType, unlockID)
	require.Len(t, unlockEvents, 2)
	require.IsType(t, &unlock.BoardsUnlockedEvent{}, unlockEvents[1])
	handleAll(t, ctx, handler, unlockEvents[1:])
	for number := 1; number <= len(publishedDecks); number++ {
		deckID := constrainttest.Owner(t, ctx, deck.StarterKey(grant.ID, startergranttest.DeckNumber(t, number)))
		handleAll(t, ctx, handler, loadEvents(t, ctx, deck.NewID, deck.NewEventType, deckID))
	}

	require.IsType(t, startergrant.CompletedStarterGrant{}, loadStarterGrant(t, ctx, playerID))
}

// 購読する集約のうち、流れの次の操作に当たらないイベント（配布の記録）では何もしない
func TestHandle_IgnoresEventsNotInFlow(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newStarterGrantHandler(t, ctx, publishedMaster())
	playerID, grantID := startedGrantID(t, ctx, handler)
	require.NoError(t, handler.RecordCollection(ctx, grantID))
	events := loadEvents(t, ctx, startergrant.NewID, startergrant.NewEventType, grantID)
	before := countEventsOf(t, ctx, starterGrantTypeID)

	handleAll(t, ctx, handler, events[1:])

	require.Equal(t, before, countEventsOf(t, ctx, starterGrantTypeID))
	require.Equal(t, 0, countEventsOf(t, ctx, collectionTypeID))
	require.IsType(t, startergrant.InProgressStarterGrant{}, loadStarterGrant(t, ctx, playerID))
}

// 解放するボードが無いセットなら、解放の集約を作ったとき（CreatedEvent を受けたとき）に解放の段を済んだものとして記録し、
// ボードの解放のイベントは出ない
func TestHandle_RecordsUnlockImmediatelyWhenStarterSetHasNoBoards(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newStarterGrantHandler(t, ctx, starterSetMaster(master.StarterSet{GrantDecks: publishedDecks, UnlockBoards: []board.ID{}}))
	playerID, grantID := startedGrantID(t, ctx, handler)
	require.NoError(t, handler.Distribute(ctx, grantID))
	grantPlayer := playertest.ID(t, playerID)
	unlockEvents := loadEvents(t, ctx, unlock.NewID, unlock.NewEventType, constrainttest.Owner(t, ctx, unlock.PlayerKey(grantPlayer)))

	handleAll(t, ctx, handler, unlockEvents)

	require.Len(t, unlockEvents, 1)
	state := loadStarterGrant(t, ctx, playerID).(startergrant.InProgressStarterGrant)
	require.True(t, state.UnlockRecorded)
}

// 配布が済んだ後に、実績などで解放したボードのイベントが来ても、何もしない
func TestHandle_IgnoresBoardsUnlockedAfterGrantIsCompleted(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newStarterGrantHandler(t, ctx, publishedMaster())
	playerID, grantID := startedGrantID(t, ctx, handler)
	require.NoError(t, handler.Distribute(ctx, grantID))
	grantPlayer := playertest.ID(t, playerID)
	unlockID := constrainttest.Owner(t, ctx, unlock.PlayerKey(grantPlayer))
	handleAll(t, ctx, handler, loadEvents(t, ctx, unlock.NewID, unlock.NewEventType, unlockID))
	handleAll(t, ctx, handler, loadEvents(t, ctx, unlock.NewID, unlock.NewEventType, unlockID)[1:])
	require.NoError(t, handler.RecordCollection(ctx, grantID))
	require.NoError(t, handler.RecordDeck(ctx, grantID, 1))
	require.NoError(t, handler.RecordDeck(ctx, grantID, 2))
	require.IsType(t, startergrant.CompletedStarterGrant{}, loadStarterGrant(t, ctx, playerID))
	before := countEventsOf(t, ctx, starterGrantTypeID)
	unlockAggregate, err := unlock.NewID(unlockID)
	require.NoError(t, err)
	unlocked, err := newUnlockHandler(t, ctx).UnlockBoards(ctx, unlockAggregate, []board.ID{boardtest.ID(t, boardC)})
	require.NoError(t, err)
	require.True(t, unlocked)

	handleAll(t, ctx, handler, loadEvents(t, ctx, unlock.NewID, unlock.NewEventType, unlockID)[2:])

	require.Equal(t, before, countEventsOf(t, ctx, starterGrantTypeID))
	require.IsType(t, startergrant.CompletedStarterGrant{}, loadStarterGrant(t, ctx, playerID))
}
