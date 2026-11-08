package repository_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room/roomtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
)

var roomBase = time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)

func issueEntity[I domain.ID](t *testing.T, f playerFixture, newID domain.NewIDFunc[I]) (I, uuid.UUID) {
	t.Helper()
	id, publicID, err := identifier.New(f.registry, newID).Issue(f.ctx)
	require.NoError(t, err)
	return id, publicID
}

// 部屋の read model の状態と、席・対戦ルールが指す公開 ID。名前を引く元の行（avatars・decks・npc_avatars・npc_decks・boards）も入れてある
type seededRoom struct {
	state                                                                           readmodel.Room
	roomPublicID, ownerPublicID                                                     uuid.UUID
	avatarPublicID, deckPublicID, npcAvatarPublicID, npcDeckPublicID, boardPublicID uuid.UUID
}

// 準備中で、プレイヤーの席（アリスと火のデッキ）と NPC の席（ゴリガンとゴリガンのデッキ）を選び、対戦ルール（ボードはチュートリアル）を決め、
// 持ち主が準備完了を申告した部屋
func seedRoom(t *testing.T, f playerFixture, owner player.ID, ownerPublicID uuid.UUID, registeredAt time.Time) seededRoom {
	t.Helper()
	roomID, roomPublicID := issueEntity(t, f, room.NewID)
	avatarID, avatarPublicID := issueEntity(t, f, avatar.NewID)
	require.NoError(t, repository.NewAvatarWriter(identifier.NewEntityRefResolver(f.registry)).Upsert(f.ctx, readmodel.Avatar{
		AvatarID: avatarPublicID, Player: owner, Name: "アリス", RegisteredAt: registeredAt,
	}))
	deckID, deckPublicID := f.issueDeck(t)
	deckMetadata, err := json.Marshal(readmodel.DeckMetadata{ID: deckPublicID, Name: "火のデッキ", Cards: []readmodel.DeckCard{}})
	require.NoError(t, err)
	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	require.NoError(t, (&models.Deck{DeckID: deckPublicID.String(), PlayerID: ownerPublicID.String(), RegisteredAt: registeredAt, Metadata: deckMetadata}).Insert(f.ctx, exec, boil.Infer()))

	npcAvatarID, npcAvatarPublicID := issueNPCAvatarEntity(t, f.ctx)
	insertNamedNPCAvatar(t, f.ctx, npcAvatarPublicID, "ゴリガン")
	npcDeckID, npcDeckPublicID := issueEntity(t, f, npcdeck.NewID)
	insertNamedNPCDeck(t, f.ctx, npcDeckPublicID, "ゴリガンのデッキ")
	boardID, boardPublicID := issueBoardEntity(t, f.ctx)
	insertNamedBoard(t, f.ctx, boardPublicID, "チュートリアル")

	playerChoice := room.NewPlayerSeatChoice(avatarID, deckID)
	npcChoice := room.NewNPCSeatChoice(npcAvatarID, npcDeckID)
	matchRules := roomtest.MatchRules(t, boardID)
	state := readmodel.Room{
		Room: roomID, Owner: owner, Name: roomtest.Name(t, "ゴリガンと対戦"), Status: readmodel.RoomStatusPreparing, Revision: 8,
		Seats: []readmodel.RoomSeat{
			{Kind: room.SeatKindPlayer, Player: owner, Choice: &playerChoice},
			{Kind: room.SeatKindNPC, Choice: &npcChoice},
		},
		MatchRules: &matchRules, Ready: []player.ID{owner},
		RegisteredAt: registeredAt, UpdatedAt: registeredAt.Add(time.Minute), UpdatedBy: owner,
	}
	return seededRoom{
		state:        state,
		roomPublicID: roomPublicID, ownerPublicID: ownerPublicID,
		avatarPublicID: avatarPublicID, deckPublicID: deckPublicID, npcAvatarPublicID: npcAvatarPublicID, npcDeckPublicID: npcDeckPublicID, boardPublicID: boardPublicID,
	}
}

func roomWriter(f playerFixture) *repository.RoomWriter {
	return repository.NewRoomWriter(identifier.NewEntityRefResolver(f.registry))
}

// 席も対戦ルールもまだ決めていない、作ったばかりの部屋
func seedEmptyRoom(t *testing.T, f playerFixture, owner player.ID, registeredAt time.Time) (readmodel.Room, uuid.UUID) {
	t.Helper()
	roomID, roomPublicID := issueEntity(t, f, room.NewID)
	state := readmodel.Room{
		Room: roomID, Owner: owner, Name: roomtest.Name(t, "作ったばかり"), Status: readmodel.RoomStatusDraft, Revision: 1, Seats: []readmodel.RoomSeat{}, Ready: []player.ID{},
		RegisteredAt: registeredAt, UpdatedAt: registeredAt, UpdatedBy: owner,
	}
	require.NoError(t, roomWriter(f).Upsert(f.ctx, state))
	return state, roomPublicID
}

func TestRoomWriter_WritesPublicIDsOfEveryInternalIDInColumnsAndMetadata(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	seeded := seedRoom(t, f, owner, ownerPublicID, roomBase)

	require.NoError(t, roomWriter(f).Upsert(f.ctx, seeded.state))

	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	row, err := models.FindRoom(f.ctx, exec, seeded.roomPublicID.String())
	require.NoError(t, err)
	require.Equal(t, ownerPublicID.String(), row.OwnerPlayerID)
	require.Equal(t, "preparing", row.Status)
	require.True(t, roomBase.Equal(row.RegisteredAt))
	var metadata readmodel.RoomMetadata
	require.NoError(t, json.Unmarshal(row.Metadata, &metadata))
	matchRules := seeded.state.MatchRules
	require.Equal(t, readmodel.RoomMetadata{
		ID: seeded.roomPublicID, Name: "ゴリガンと対戦", Status: readmodel.RoomStatusPreparing, Revision: 8, Owner: ownerPublicID,
		Seats: []readmodel.RoomSeatMetadata{
			{Kind: room.SeatKindPlayer, Player: &ownerPublicID, Avatar: &seeded.avatarPublicID, Deck: &seeded.deckPublicID},
			{Kind: room.SeatKindNPC, Avatar: &seeded.npcAvatarPublicID, Deck: &seeded.npcDeckPublicID},
		},
		MatchRules: &readmodel.RoomMatchRulesMetadata{
			Board: seeded.boardPublicID, TargetMagic: matchRules.TargetMagic, StartingMagic: matchRules.StartingMagic, LapBonus: matchRules.LapBonus,
			CheckpointBonus: matchRules.CheckpointBonus, RoundLimit: matchRules.RoundLimit, DiceMin: matchRules.DiceMin, DiceMax: matchRules.DiceMax, BaseLandValue: matchRules.BaseLandValue,
		},
		ReadyPlayers: []uuid.UUID{ownerPublicID}, Matches: []uuid.UUID{}, NextDeclarations: []readmodel.RoomNextDeclarationMetadata{},
		RegisteredAt: roomBase, UpdatedAt: roomBase.Add(time.Minute), RegisteredBy: ownerPublicID, UpdatedBy: ownerPublicID,
	}, metadata)
}

func TestRoomWriter_WritesUndecidedConditionsAsEmpty(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	state, roomPublicID := seedEmptyRoom(t, f, owner, roomBase)

	got, err := repository.NewRoomReader().Get(f.ctx, ownerPublicID, roomPublicID)

	require.NoError(t, err)
	require.Equal(t, state.Revision, got.Revision)
	require.Equal(t, query.RoomStatusDraft, got.Status)
	require.Equal(t, []query.RoomSeat{}, got.Seats)
	require.Nil(t, got.MatchRules)
	require.Equal(t, []query.RoomParticipant{{PlayerID: ownerPublicID, Owner: true}}, got.Participants)
}

func TestRoomWriter_UpsertTwiceKeepsOneRowWithLatestState(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	seeded := seedRoom(t, f, owner, ownerPublicID, roomBase)
	writer := roomWriter(f)
	require.NoError(t, writer.Upsert(f.ctx, seeded.state))

	closed := seeded.state
	closed.Status, closed.Revision = readmodel.RoomStatusClosed, 9
	require.NoError(t, writer.Upsert(f.ctx, closed))

	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	count, err := models.Rooms().Count(f.ctx, exec)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	row, err := models.FindRoom(f.ctx, exec, seeded.roomPublicID.String())
	require.NoError(t, err)
	require.Equal(t, "closed", row.Status)
}

func TestRoomWriter_FailsWhenAnInternalIDHasNoPublicID(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	seeded := seedRoom(t, f, owner, ownerPublicID, roomBase)
	matchRules := roomtest.MatchRules(t, boardtest.ID(t, 999999))
	seeded.state.MatchRules = &matchRules

	err := roomWriter(f).Upsert(f.ctx, seeded.state)

	require.Error(t, err)
}

func TestRoomWriter_DeleteRemovesRowAndIgnoresMissingRow(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	seeded := seedRoom(t, f, owner, ownerPublicID, roomBase)
	writer := roomWriter(f)
	require.NoError(t, writer.Upsert(f.ctx, seeded.state))

	require.NoError(t, writer.Delete(f.ctx, seeded.state))
	require.NoError(t, writer.Delete(f.ctx, seeded.state))

	_, err := repository.NewRoomReader().Get(f.ctx, ownerPublicID, seeded.roomPublicID)
	require.ErrorIs(t, err, query.ErrRoomNotFound)
}

func TestRoomReader_GetReturnsRoomWithNamesJoinedAtReadTime(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	seeded := seedRoom(t, f, owner, ownerPublicID, roomBase)
	require.NoError(t, roomWriter(f).Upsert(f.ctx, seeded.state))

	got, err := repository.NewRoomReader().Get(f.ctx, ownerPublicID, seeded.roomPublicID)

	require.NoError(t, err)
	rules := seeded.state.MatchRules
	boardNamed := query.RoomNamed{ID: seeded.boardPublicID, Name: "チュートリアル"}
	require.Equal(t, query.Room{
		ID: seeded.roomPublicID, Name: "ゴリガンと対戦", Revision: 8, Status: query.RoomStatusPreparing,
		Participants: []query.RoomParticipant{{PlayerID: ownerPublicID, Owner: true, Ready: true}}, Matches: []uuid.UUID{},
		Seats: []query.RoomSeat{
			{Kind: query.RoomSeatKindPlayer, Player: &ownerPublicID, Avatar: &query.RoomNamed{ID: seeded.avatarPublicID, Name: "アリス"}, Deck: &query.RoomNamed{ID: seeded.deckPublicID, Name: "火のデッキ"}},
			{Kind: query.RoomSeatKindNPC, Avatar: &query.RoomNamed{ID: seeded.npcAvatarPublicID, Name: "ゴリガン"}, Deck: &query.RoomNamed{ID: seeded.npcDeckPublicID, Name: "ゴリガンのデッキ"}},
		},
		MatchRules: &query.RoomMatchRules{
			Board:       boardNamed,
			TargetMagic: rules.TargetMagic, StartingMagic: rules.StartingMagic, LapBonus: rules.LapBonus, CheckpointBonus: rules.CheckpointBonus,
			RoundLimit: rules.RoundLimit, DiceMin: rules.DiceMin, DiceMax: rules.DiceMax, BaseLandValue: rules.BaseLandValue,
		},
	}, got)
}

func TestRoomWriterAndReader_PlayingRoomCarriesPublicIDOfMatch(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	seeded := seedRoom(t, f, owner, ownerPublicID, roomBase)
	matchID, matchPublicID := issueEntity(t, f, match.NewID)
	state := seeded.state
	state.Status, state.Match = readmodel.RoomStatusPlaying, matchID
	require.NoError(t, roomWriter(f).Upsert(f.ctx, state))

	got, err := repository.NewRoomReader().Get(f.ctx, ownerPublicID, seeded.roomPublicID)

	require.NoError(t, err)
	require.Equal(t, query.RoomStatusPlaying, got.Status)
	require.Equal(t, &matchPublicID, got.Match)
}

func TestRoomWriterAndReader_RoomCarriesStartedMatchesInOrderAndDeclarationsOfNext(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	seeded := seedRoom(t, f, owner, ownerPublicID, roomBase)
	first, firstPublicID := issueEntity(t, f, match.NewID)
	second, secondPublicID := issueEntity(t, f, match.NewID)
	state := seeded.state
	state.Status, state.Match, state.Matches = readmodel.RoomStatusDecidingNext, second, []match.ID{first, second}
	state.NextDeclarations = []readmodel.RoomNextDeclaration{{Player: owner, Continue: true}}
	require.NoError(t, roomWriter(f).Upsert(f.ctx, state))

	got, err := repository.NewRoomReader().Get(f.ctx, ownerPublicID, seeded.roomPublicID)

	require.NoError(t, err)
	require.Equal(t, query.RoomStatusDecidingNext, got.Status)
	require.Equal(t, []uuid.UUID{firstPublicID, secondPublicID}, got.Matches)
	require.Equal(t, []query.RoomParticipant{{PlayerID: ownerPublicID, Owner: true, Ready: true, Continue: new(true)}}, got.Participants)
}

func TestRoomReader_FindByMatchReturnsRoomAndNumberOfTheMatch(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	seeded := seedRoom(t, f, owner, ownerPublicID, roomBase)
	first, firstPublicID := issueEntity(t, f, match.NewID)
	second, secondPublicID := issueEntity(t, f, match.NewID)
	state := seeded.state
	state.Matches = []match.ID{first, second}
	require.NoError(t, roomWriter(f).Upsert(f.ctx, state))
	otherOwner, otherOwnerPublicID := f.issuePlayer(t)
	other := seedRoom(t, f, otherOwner, otherOwnerPublicID, roomBase)
	third, thirdPublicID := issueEntity(t, f, match.NewID)
	otherState := other.state
	otherState.Matches = []match.ID{third}
	require.NoError(t, roomWriter(f).Upsert(f.ctx, otherState))
	reader := repository.NewRoomReader()

	roomID1, number1, found1, err1 := reader.FindByMatch(f.ctx, firstPublicID)
	roomID2, number2, found2, err2 := reader.FindByMatch(f.ctx, secondPublicID)
	roomID3, number3, found3, err3 := reader.FindByMatch(f.ctx, thirdPublicID)

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.NoError(t, err3)
	require.True(t, found1 && found2 && found3)
	require.Equal(t, seeded.roomPublicID, roomID1)
	require.Equal(t, 1, number1)
	require.Equal(t, seeded.roomPublicID, roomID2)
	require.Equal(t, 2, number2)
	require.Equal(t, other.roomPublicID, roomID3)
	require.Equal(t, 1, number3)
}

func TestRoomReader_FindByMatchReportsNotFoundForMatchOfNoRoom(t *testing.T) {
	f := newPlayerFixture(t)
	_, unknown := issueEntity(t, f, match.NewID)

	_, _, found, err := repository.NewRoomReader().FindByMatch(f.ctx, unknown)

	require.NoError(t, err)
	require.False(t, found)
}

func TestRoomReader_GetReturnsNotFoundForRoomOfOtherPlayerAndUnknownRoom(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	_, otherPublicID := f.issuePlayer(t)
	seeded := seedRoom(t, f, owner, ownerPublicID, roomBase)
	require.NoError(t, roomWriter(f).Upsert(f.ctx, seeded.state))

	_, ofOther := repository.NewRoomReader().Get(f.ctx, otherPublicID, seeded.roomPublicID)
	_, unknown := repository.NewRoomReader().Get(f.ctx, ownerPublicID, uuid.New())

	require.ErrorIs(t, ofOther, query.ErrRoomNotFound)
	require.ErrorIs(t, unknown, query.ErrRoomNotFound)
}

func TestRoomReader_GetReturnsClosedRoom(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	seeded := seedRoom(t, f, owner, ownerPublicID, roomBase)
	closed := seeded.state
	closed.Status = readmodel.RoomStatusClosed
	require.NoError(t, roomWriter(f).Upsert(f.ctx, closed))

	got, err := repository.NewRoomReader().Get(f.ctx, ownerPublicID, seeded.roomPublicID)

	require.NoError(t, err)
	require.Equal(t, query.RoomStatusClosed, got.Status)
}

func TestRoomReader_GetFailsWhenAJoinedNameIsMissing(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	seeded := seedRoom(t, f, owner, ownerPublicID, roomBase)
	require.NoError(t, roomWriter(f).Upsert(f.ctx, seeded.state))
	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	_, err = models.Boards(models.BoardWhere.BoardID.EQ(seeded.boardPublicID.String())).DeleteAll(f.ctx, exec)
	require.NoError(t, err)

	_, err = repository.NewRoomReader().Get(f.ctx, ownerPublicID, seeded.roomPublicID)

	require.Error(t, err)
}

// 新しい順に読み、閉じた部屋とほかのプレイヤーの部屋は含めない。続きは行の比べ方で取る
func TestRoomReader_ListByPlayerReturnsOwnOpenRoomsNewestFirstAndContinuesAfterCursor(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	other, otherPublicID := f.issuePlayer(t)
	_, oldest := seedEmptyRoom(t, f, owner, roomBase)
	_, middle := seedEmptyRoom(t, f, owner, roomBase.Add(time.Second))
	_, newest := seedEmptyRoom(t, f, owner, roomBase.Add(2*time.Second))
	seededClosed := seedRoom(t, f, owner, ownerPublicID, roomBase.Add(3*time.Second))
	closed := seededClosed.state
	closed.Status = readmodel.RoomStatusClosed
	require.NoError(t, roomWriter(f).Upsert(f.ctx, closed))
	_, _ = seedEmptyRoom(t, f, other, roomBase.Add(4*time.Second))
	reader := repository.NewRoomReader()

	all, err := reader.ListByPlayer(f.ctx, ownerPublicID, query.ListRoomsCondition{Limit: 10})
	require.NoError(t, err)
	next, err := reader.ListByPlayer(f.ctx, ownerPublicID, query.ListRoomsCondition{After: &all[0].Cursor, Limit: 10})
	require.NoError(t, err)
	others, err := reader.ListByPlayer(f.ctx, otherPublicID, query.ListRoomsCondition{Limit: 10})
	require.NoError(t, err)
	limited, err := reader.ListByPlayer(f.ctx, ownerPublicID, query.ListRoomsCondition{Limit: 2})
	require.NoError(t, err)

	require.Equal(t, []uuid.UUID{newest, middle, oldest}, roomIDs(all))
	require.Equal(t, []uuid.UUID{middle, oldest}, roomIDs(next))
	require.Len(t, others, 1)
	require.Equal(t, []uuid.UUID{newest, middle}, roomIDs(limited))
	require.Equal(t, query.RoomListCursor{RegisteredAt: roomBase.Add(2 * time.Second), RoomID: newest}, all[0].Cursor)
}

func roomIDs(rows []query.RoomListRow) []uuid.UUID {
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.Room.ID
	}
	return ids
}

func TestRoomReader_ListByPlayerReturnsNothingWhenThereIsNoRoom(t *testing.T) {
	f := newPlayerFixture(t)
	_, ownerPublicID := f.issuePlayer(t)

	got, err := repository.NewRoomReader().ListByPlayer(f.ctx, ownerPublicID, query.ListRoomsCondition{Limit: 10})

	require.NoError(t, err)
	require.Empty(t, got)
}

func TestRoomReader_ListByPlayerJoinsNamesOfEveryRoomOnce(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	first := seedRoom(t, f, owner, ownerPublicID, roomBase)
	second := seedRoom(t, f, owner, ownerPublicID, roomBase.Add(time.Second))
	require.NoError(t, roomWriter(f).Upsert(f.ctx, first.state))
	require.NoError(t, roomWriter(f).Upsert(f.ctx, second.state))

	got, err := repository.NewRoomReader().ListByPlayer(f.ctx, ownerPublicID, query.ListRoomsCondition{Limit: 10})

	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, second.roomPublicID, got[0].Room.ID)
	require.Equal(t, "ゴリガン", got[0].Room.Seats[1].Avatar.Name)
	require.Equal(t, "チュートリアル", got[1].Room.MatchRules.Board.Name)
}
