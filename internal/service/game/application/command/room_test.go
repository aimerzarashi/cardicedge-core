package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room/roomtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/rules"
)

// 画面の選択肢を出す query の代わりに、選べるものを決めた値で答える。誰について調べたかも覚える
type fakeSelectable struct {
	avatars, decks, npcAvatars, boards map[uuid.UUID]bool
	// NPC のアバターごとの、割り当てた公開中のデッキ
	npcDecks map[uuid.UUID]map[uuid.UUID]bool
	askedFor []uuid.UUID
}

func (s *fakeSelectable) HasAvatar(_ context.Context, playerID uuid.UUID, avatarID uuid.UUID) (bool, error) {
	s.askedFor = append(s.askedFor, playerID)
	return s.avatars[avatarID], nil
}

func (s *fakeSelectable) HasDeck(_ context.Context, playerID uuid.UUID, deckID uuid.UUID) (bool, error) {
	s.askedFor = append(s.askedFor, playerID)
	return s.decks[deckID], nil
}

func (s *fakeSelectable) HasNPCAvatar(_ context.Context, playerID uuid.UUID, npcAvatarID uuid.UUID) (bool, error) {
	s.askedFor = append(s.askedFor, playerID)
	return s.npcAvatars[npcAvatarID], nil
}

func (s *fakeSelectable) HasNPCDeck(_ context.Context, playerID uuid.UUID, npcAvatarID uuid.UUID, npcDeckID uuid.UUID) (bool, error) {
	s.askedFor = append(s.askedFor, playerID)
	return s.npcAvatars[npcAvatarID] && s.npcDecks[npcAvatarID][npcDeckID], nil
}

func (s *fakeSelectable) HasBoard(_ context.Context, playerID uuid.UUID, boardID uuid.UUID) (bool, error) {
	s.askedFor = append(s.askedFor, playerID)
	return s.boards[boardID], nil
}

type roomFixture struct {
	ctx        context.Context
	owner      context.Context
	handler    *command.RoomHandler
	selectable *fakeSelectable
	registry   *domaindef.Registry
	// 選べるもの（公開 ID）
	avatar, deck, npcAvatar, npcDeck, board uuid.UUID
}

func newRoomFixture(t *testing.T) roomFixture {
	t.Helper()
	return newRoomFixtureWithRules(t, func(context.Context) (gamerules.Rules, bool, error) { return gamerules.Defaults(), true, nil })
}

func newRoomFixtureWithRules(t *testing.T, getRules rules.GetRulesFunc) roomFixture {
	t.Helper()
	ctx := isolatedDatabase(t)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	f := roomFixture{
		ctx: ctx, owner: registeredPlayer(t, ctx, "owner"), registry: registry,
		selectable: &fakeSelectable{
			avatars: map[uuid.UUID]bool{}, decks: map[uuid.UUID]bool{}, npcAvatars: map[uuid.UUID]bool{}, boards: map[uuid.UUID]bool{},
			npcDecks: map[uuid.UUID]map[uuid.UUID]bool{},
		},
	}
	f.avatar = selectable(t, ctx, registry, avatar.NewID, f.selectable.avatars)
	f.deck = selectable(t, ctx, registry, deck.NewID, f.selectable.decks)
	f.npcAvatar = selectable(t, ctx, registry, npcavatar.NewID, f.selectable.npcAvatars)
	f.board = selectable(t, ctx, registry, board.NewID, f.selectable.boards)
	f.npcDeck = selectable(t, ctx, registry, npcdeck.NewID, map[uuid.UUID]bool{})
	f.selectable.npcDecks[f.npcAvatar] = map[uuid.UUID]bool{f.npcDeck: true}
	f.handler = command.NewRoomHandler(
		eventstore.New(registry, room.NewID, room.NewEventType, room.InitialState, room.Evolve),
		identifier.New(registry, room.NewID),
		command.RoomReferences{
			Avatars: identifier.New(registry, avatar.NewID), Decks: identifier.New(registry, deck.NewID),
			NPCAvatars: identifier.New(registry, npcavatar.NewID), NPCDecks: identifier.New(registry, npcdeck.NewID),
			Boards: identifier.New(registry, board.NewID),
		},
		identifier.NewEntityRefResolver(registry),
		f.selectable,
		getRules,
		func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	)
	return f
}

// 識別子を発行し、選べるものの集合に入れる（game の domain_entities の行は、publish-master や配布の流れが作る）
func selectable[I domain.ID](t *testing.T, ctx context.Context, registry *domaindef.Registry, newID domain.NewIDFunc[I], set map[uuid.UUID]bool) uuid.UUID {
	t.Helper()
	_, publicID, err := identifier.New(registry, newID).Issue(ctx)
	require.NoError(t, err)
	set[publicID] = true
	return publicID
}

func (f roomFixture) load(t *testing.T, publicID uuid.UUID) room.Room {
	t.Helper()
	id, err := identifier.New(f.registry, room.NewID).Resolve(f.ctx, publicID)
	require.NoError(t, err)
	state, err := eventstore.New(f.registry, room.NewID, room.NewEventType, room.InitialState, room.Evolve).Load(f.ctx, id)
	require.NoError(t, err)
	loaded, ok := state.(room.Room)
	require.True(t, ok)
	return loaded
}

func (f roomFixture) create(t *testing.T) command.ChangedRoom {
	t.Helper()
	created, err := f.handler.Create(f.owner, "はじめての部屋")
	require.NoError(t, err)
	return created
}

var playerAndNPC = []command.RoomSeatKind{command.RoomSeatKindPlayer, command.RoomSeatKindNPC}

// 席の並びを プレイヤー → NPC に決めた部屋
func (f roomFixture) withSeats(t *testing.T) command.ChangedRoom {
	t.Helper()
	created := f.create(t)
	changed, err := f.handler.ChangeSeats(f.owner, created.ID, created.Revision, playerAndNPC)
	require.NoError(t, err)
	return changed
}

func (f roomFixture) rulesInput() command.RoomMatchRulesInput {
	return command.RoomMatchRulesInput{
		BoardID: f.board, TargetMagic: 8000, StartingMagic: 600, LapBonus: 300, CheckpointBonus: 100, RoundLimit: 30, DiceMin: 1, DiceMax: 6,
	}
}

// 対戦ルールと 2 つの席の選択まで決めた部屋
func (f roomFixture) fullyDecided(t *testing.T) command.ChangedRoom {
	t.Helper()
	current := f.withSeats(t)
	current, err := f.handler.ChangeMatchRules(f.owner, current.ID, current.Revision, f.rulesInput())
	require.NoError(t, err)
	current, err = f.handler.ChangeSeatChoice(f.owner, current.ID, current.Revision, 0, f.avatar, f.deck)
	require.NoError(t, err)
	current, err = f.handler.ChangeSeatChoice(f.owner, current.ID, current.Revision, 1, f.npcAvatar, f.npcDeck)
	require.NoError(t, err)
	return current
}

func TestRoomCreate_MakesDraftRoomOfPlayerWithoutSeats(t *testing.T) {
	f := newRoomFixture(t)

	created, err := f.handler.Create(f.owner, "はじめての部屋")

	require.NoError(t, err)
	require.Equal(t, int64(1), created.Revision)
	loaded := f.load(t, created.ID)
	require.Equal(t, room.StatusDraft, loaded.Status)
	require.Equal(t, "はじめての部屋", loaded.Name.String())
	require.Empty(t, loaded.Seats)
	require.Equal(t, 1, countEventsOf(t, f.ctx, roomTypeID))
	other := f.create(t)
	require.NotEqual(t, created.ID, other.ID)
}

const roomTypeID = 2900

func TestRoomHandler_RejectsUnregisteredPlayer(t *testing.T) {
	f := newRoomFixture(t)
	created := f.create(t)
	operations := []struct {
		name string
		call func() error
	}{
		{name: "create", call: func() error { _, err := f.handler.Create(f.ctx, "はじめての部屋"); return err }},
		{name: "change_seats", call: func() error {
			_, err := f.handler.ChangeSeats(f.ctx, created.ID, created.Revision, playerAndNPC)
			return err
		}},
		{name: "publish", call: func() error { _, err := f.handler.Publish(f.ctx, created.ID, created.Revision); return err }},
	}
	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			require.ErrorIs(t, op.call(), command.ErrPlayerNotRegistered)
		})
	}
}

// 入っていない部屋・無い部屋は、存在を明かさないため同じ ErrRoomNotFound にする
func TestRoomHandler_ReturnsNotFoundForUnknownRoomAndRoomOfOthers(t *testing.T) {
	f := newRoomFixture(t)
	created := f.create(t)
	stranger := registeredPlayer(t, f.ctx, "stranger")

	_, unknown := f.handler.Publish(f.owner, uuid.New(), 1)
	_, notMember := f.handler.Close(stranger, created.ID, created.Revision)
	_, notMemberSeats := f.handler.ChangeSeats(stranger, created.ID, created.Revision, playerAndNPC)

	require.ErrorIs(t, unknown, command.ErrRoomNotFound)
	require.ErrorIs(t, notMember, command.ErrRoomNotFound)
	require.ErrorIs(t, notMemberSeats, command.ErrRoomNotFound)
	require.Equal(t, room.StatusDraft, f.load(t, created.ID).Status)
}

func TestRoomHandler_RejectsStaleRevisionOnEveryOperation(t *testing.T) {
	f := newRoomFixture(t)
	current := f.withSeats(t)
	stale := current.Revision - 1
	operations := []struct {
		name string
		call func() error
	}{
		{name: "change_seats", call: func() error { _, err := f.handler.ChangeSeats(f.owner, current.ID, stale, playerAndNPC); return err }},
		{name: "change_match_rules", call: func() error {
			_, err := f.handler.ChangeMatchRules(f.owner, current.ID, stale, f.rulesInput())
			return err
		}},
		{name: "change_seat_choice", call: func() error {
			_, err := f.handler.ChangeSeatChoice(f.owner, current.ID, stale, 0, f.avatar, f.deck)
			return err
		}},
		{name: "publish", call: func() error { _, err := f.handler.Publish(f.owner, current.ID, stale); return err }},
		{name: "declare_ready", call: func() error { _, err := f.handler.DeclareReady(f.owner, current.ID, stale); return err }},
		{name: "close", call: func() error { _, err := f.handler.Close(f.owner, current.ID, stale); return err }},
		{name: "rename", call: func() error { _, err := f.handler.Rename(f.owner, current.ID, stale, "新しい名前"); return err }},
	}
	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			require.ErrorIs(t, op.call(), command.ErrRevisionConflict)
		})
	}
	require.Equal(t, current.Revision, f.load(t, current.ID).Version)
}

func TestRoomChangeSeats_DecidesSeatsAndKeepsRevisionForSameSeats(t *testing.T) {
	f := newRoomFixture(t)
	created := f.create(t)

	changed, err := f.handler.ChangeSeats(f.owner, created.ID, created.Revision, playerAndNPC)
	again, againErr := f.handler.ChangeSeats(f.owner, created.ID, changed.Revision, playerAndNPC)

	require.NoError(t, err)
	require.NoError(t, againErr)
	require.Equal(t, command.ChangedRoom{ID: created.ID, Revision: 2}, changed)
	require.Equal(t, changed, again)
	seats := f.load(t, created.ID).Seats
	require.Len(t, seats, 2)
	require.Equal(t, room.SeatKindPlayer, seats[0].Seat.Kind)
	require.NotZero(t, seats[0].Seat.Player)
	require.Equal(t, room.SeatKindNPC, seats[1].Seat.Kind)
}

func TestRoomChangeSeats_ReturnsInputErrorsInStages(t *testing.T) {
	player, npc := command.RoomSeatKindPlayer, command.RoomSeatKindNPC
	tests := []struct {
		name  string
		seats []command.RoomSeatKind
		want  []input.Error
	}{
		{name: "too_few_seats", seats: []command.RoomSeatKind{player}, want: []input.Error{&input.OutOfRange{Field: "seats", Min: 2, Max: 4}}},
		{name: "too_many_seats", seats: []command.RoomSeatKind{player, npc, npc, npc, npc}, want: []input.Error{&input.OutOfRange{Field: "seats", Min: 2, Max: 4}}},
		{name: "no_player_seat", seats: []command.RoomSeatKind{npc, npc}, want: []input.Error{&input.CountMismatch{Field: "seats", Expected: 1}}},
		{name: "two_player_seats", seats: []command.RoomSeatKind{player, player, npc}, want: []input.Error{&input.CountMismatch{Field: "seats", Expected: 1}}},
		// 席の数が範囲の外なら、プレイヤーの席の数は確かめない
		{name: "seat_count_is_checked_first", seats: []command.RoomSeatKind{npc, npc, npc, npc, npc}, want: []input.Error{&input.OutOfRange{Field: "seats", Min: 2, Max: 4}}},
		{name: "unknown_kind", seats: []command.RoomSeatKind{player, "team"}, want: []input.Error{&input.InvalidFormat{Field: "seats.1"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRoomFixture(t)
			created := f.create(t)

			_, err := f.handler.ChangeSeats(f.owner, created.ID, created.Revision, tt.seats)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.Empty(t, f.load(t, created.ID).Seats)
		})
	}
}

func TestRoomChangeSeats_UsesPublishedGameRulesRange(t *testing.T) {
	f := newRoomFixtureWithRules(t, func(context.Context) (gamerules.Rules, bool, error) {
		values := gamerules.Defaults()
		values.Match = gamerulestestMatch(t, 3, 3)
		return values, true, nil
	})
	created := f.create(t)

	_, err := f.handler.ChangeSeats(f.owner, created.ID, created.Revision, playerAndNPC)

	invalid, _ := input.AsErrors(err)
	require.Equal(t, input.Errors{&input.OutOfRange{Field: "seats", Min: 3, Max: 3}}, invalid)
}

func TestRoomChangeSeats_ReturnsErrNotPublishedWhenGameRulesAreNotPublished(t *testing.T) {
	f := newRoomFixtureWithRules(t, func(context.Context) (gamerules.Rules, bool, error) { return gamerules.Rules{}, false, nil })
	created := f.create(t)

	_, err := f.handler.ChangeSeats(f.owner, created.ID, created.Revision, playerAndNPC)

	require.ErrorIs(t, err, rules.ErrNotPublished)
}

func TestRoomChangeMatchRules_DecidesRulesOfSelectableBoard(t *testing.T) {
	f := newRoomFixture(t)
	created := f.create(t)

	changed, err := f.handler.ChangeMatchRules(f.owner, created.ID, created.Revision, f.rulesInput())

	require.NoError(t, err)
	require.Equal(t, int64(2), changed.Revision)
	loaded := f.load(t, created.ID)
	require.NotNil(t, loaded.MatchRules)
	require.Equal(t, 8000, loaded.MatchRules.TargetMagic)
	require.Equal(t, 6, loaded.MatchRules.DiceMax)
	// ボードは作った人が解放したものか調べる
	require.NotEmpty(t, f.selectable.askedFor)
}

func TestRoomChangeMatchRules_ReturnsInputErrorsForValuesAndBoardTogether(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*command.RoomMatchRulesInput)
		want   []input.Error
	}{
		{name: "board_not_selectable", modify: func(in *command.RoomMatchRulesInput) { in.BoardID = uuid.New() }, want: []input.Error{&input.NotFound{Field: "boardId"}}},
		// 目標の魔力が 1 未満なら、開始時の魔力より大きくもない（board.NewDefaultRules が項目ごとにまとめて返す）
		{name: "target_magic_too_small", modify: func(in *command.RoomMatchRulesInput) { in.TargetMagic = 0 }, want: []input.Error{&input.TooSmall{Field: "targetMagic", Min: 1}, &input.NotGreaterThan{Field: "targetMagic", Than: "startingMagic"}}},
		{name: "starting_magic_negative", modify: func(in *command.RoomMatchRulesInput) { in.StartingMagic = -1 }, want: []input.Error{&input.TooSmall{Field: "startingMagic", Min: 0}}},
		{name: "target_magic_not_greater_than_starting_magic", modify: func(in *command.RoomMatchRulesInput) { in.TargetMagic = 600 }, want: []input.Error{&input.NotGreaterThan{Field: "targetMagic", Than: "startingMagic"}}},
		{name: "dice_max_less_than_dice_min", modify: func(in *command.RoomMatchRulesInput) { in.DiceMax = 0 }, want: []input.Error{&input.LessThan{Field: "diceMax", Than: "diceMin"}}},
		{name: "value_and_board_together", modify: func(in *command.RoomMatchRulesInput) { in.RoundLimit = 0; in.BoardID = uuid.New() }, want: []input.Error{&input.TooSmall{Field: "roundLimit", Min: 1}, &input.NotFound{Field: "boardId"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRoomFixture(t)
			created := f.create(t)
			rulesInput := f.rulesInput()
			tt.modify(&rulesInput)

			_, err := f.handler.ChangeMatchRules(f.owner, created.ID, created.Revision, rulesInput)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.Nil(t, f.load(t, created.ID).MatchRules)
		})
	}
}

func TestRoomChangeSeatChoice_DecidesChoicesOfPlayerSeatAndNPCSeat(t *testing.T) {
	f := newRoomFixture(t)
	current := f.withSeats(t)

	current, err := f.handler.ChangeSeatChoice(f.owner, current.ID, current.Revision, 0, f.avatar, f.deck)
	require.NoError(t, err)
	current, err = f.handler.ChangeSeatChoice(f.owner, current.ID, current.Revision, 1, f.npcAvatar, f.npcDeck)
	require.NoError(t, err)

	require.Equal(t, int64(4), current.Revision)
	seats := f.load(t, current.ID).Seats
	require.NotNil(t, seats[0].Choice.Player)
	require.NotNil(t, seats[1].Choice.NPC)
	avatarID, err := identifier.New(f.registry, avatar.NewID).Resolve(f.ctx, f.avatar)
	require.NoError(t, err)
	require.Equal(t, avatarID, seats[0].Choice.Player.Avatar)
	npcDeckID, err := identifier.New(f.registry, npcdeck.NewID).Resolve(f.ctx, f.npcDeck)
	require.NoError(t, err)
	require.Equal(t, npcDeckID, seats[1].Choice.NPC.Deck)
}

func TestRoomChangeSeatChoice_ReturnsSeatErrors(t *testing.T) {
	f := newRoomFixture(t)
	noSeats := f.create(t)
	current := f.withSeats(t)

	_, undecided := f.handler.ChangeSeatChoice(f.owner, noSeats.ID, noSeats.Revision, 0, f.avatar, f.deck)
	_, outOfRange := f.handler.ChangeSeatChoice(f.owner, current.ID, current.Revision, 2, f.avatar, f.deck)
	_, negative := f.handler.ChangeSeatChoice(f.owner, current.ID, current.Revision, -1, f.avatar, f.deck)

	require.ErrorIs(t, undecided, command.ErrRoomSeatsNotDecided)
	for _, err := range []error{outOfRange, negative} {
		invalid, ok := input.AsErrors(err)
		require.True(t, ok)
		require.Equal(t, input.Errors{&input.OutOfRange{Field: "seat", Min: 0, Max: 1}}, invalid)
	}
}

func TestRoomChangeSeatChoice_ReturnsNotFoundForUnselectableValues(t *testing.T) {
	tests := []struct {
		name   string
		seat   int
		avatar func(f roomFixture) uuid.UUID
		deck   func(f roomFixture) uuid.UUID
		want   []input.Error
	}{
		{name: "player_seat_avatar_not_own", seat: 0, avatar: unknownID, deck: func(f roomFixture) uuid.UUID { return f.deck }, want: []input.Error{&input.NotFound{Field: "avatarId"}}},
		{name: "player_seat_deck_not_own", seat: 0, avatar: func(f roomFixture) uuid.UUID { return f.avatar }, deck: unknownID, want: []input.Error{&input.NotFound{Field: "deckId"}}},
		{name: "player_seat_both_not_own", seat: 0, avatar: unknownID, deck: unknownID, want: []input.Error{&input.NotFound{Field: "avatarId"}, &input.NotFound{Field: "deckId"}}},
		// プレイヤーのアバターは NPC の席に選べない
		{name: "npc_seat_player_avatar", seat: 1, avatar: func(f roomFixture) uuid.UUID { return f.avatar }, deck: func(f roomFixture) uuid.UUID { return f.npcDeck }, want: []input.Error{&input.NotFound{Field: "avatarId"}}},
		// NPC のアバターが選べないときは、デッキを確かめない
		{name: "npc_seat_avatar_not_unlocked_skips_deck", seat: 1, avatar: unknownID, deck: unknownID, want: []input.Error{&input.NotFound{Field: "avatarId"}}},
		{name: "npc_seat_deck_not_assigned", seat: 1, avatar: func(f roomFixture) uuid.UUID { return f.npcAvatar }, deck: func(f roomFixture) uuid.UUID { return f.deck }, want: []input.Error{&input.NotFound{Field: "deckId"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRoomFixture(t)
			current := f.withSeats(t)

			_, err := f.handler.ChangeSeatChoice(f.owner, current.ID, current.Revision, tt.seat, tt.avatar(f), tt.deck(f))

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.Nil(t, f.load(t, current.ID).Seats[tt.seat].Choice)
		})
	}
}

func unknownID(roomFixture) uuid.UUID {
	return uuid.New()
}

func TestRoomPublish_MovesToPreparingAndLocksWhatWasDecided(t *testing.T) {
	f := newRoomFixture(t)
	current := f.fullyDecided(t)

	published, err := f.handler.Publish(f.owner, current.ID, current.Revision)

	require.NoError(t, err)
	require.Equal(t, current.Revision+2, published.Revision)
	loaded := f.load(t, current.ID)
	require.Equal(t, room.StatusPreparing, loaded.Status)
	require.True(t, loaded.MatchRulesLocked)
	require.True(t, loaded.Seats[0].ChoiceLocked)
	_, changeSeats := f.handler.ChangeSeats(f.owner, current.ID, published.Revision, playerAndNPC)
	_, changeRules := f.handler.ChangeMatchRules(f.owner, current.ID, published.Revision, f.rulesInput())
	_, changeChoice := f.handler.ChangeSeatChoice(f.owner, current.ID, published.Revision, 0, f.avatar, f.deck)
	_, publishAgain := f.handler.Publish(f.owner, current.ID, published.Revision)
	require.ErrorIs(t, changeSeats, command.ErrRoomStatusNotAllowed)
	require.ErrorIs(t, changeRules, command.ErrRoomConditionLocked)
	require.ErrorIs(t, changeChoice, command.ErrRoomConditionLocked)
	require.ErrorIs(t, publishAgain, command.ErrRoomStatusNotAllowed)
}

func TestRoomPublish_RequiresDecidedSeatsButNotChoices(t *testing.T) {
	f := newRoomFixture(t)
	noSeats := f.create(t)
	withSeats := f.withSeats(t)

	_, undecided := f.handler.Publish(f.owner, noSeats.ID, noSeats.Revision)
	_, ok := f.handler.Publish(f.owner, withSeats.ID, withSeats.Revision)

	require.ErrorIs(t, undecided, command.ErrRoomSeatsNotDecided)
	require.NoError(t, ok)
}

// 公開前に決めなかったものを準備中に決め、準備完了の申告まで選び直せる。申告の後は自分の担当の分を変えられない
func TestRoomDeclareReady_DecidesRestInPreparingAndLocksOwnChoicesAfterReady(t *testing.T) {
	f := newRoomFixture(t)
	current := f.withSeats(t)
	published, err := f.handler.Publish(f.owner, current.ID, current.Revision)
	require.NoError(t, err)

	_, notDecided := f.handler.DeclareReady(f.owner, current.ID, published.Revision)
	next, err := f.handler.ChangeMatchRules(f.owner, current.ID, published.Revision, f.rulesInput())
	require.NoError(t, err)
	next, err = f.handler.ChangeSeatChoice(f.owner, current.ID, next.Revision, 0, f.avatar, f.deck)
	require.NoError(t, err)
	next, err = f.handler.ChangeSeatChoice(f.owner, current.ID, next.Revision, 1, f.npcAvatar, f.npcDeck)
	require.NoError(t, err)
	// 準備完了までは選び直せる
	rechosen := f.rulesInput()
	rechosen.TargetMagic = 9000
	next, err = f.handler.ChangeMatchRules(f.owner, current.ID, next.Revision, rechosen)
	require.NoError(t, err)
	ready, err := f.handler.DeclareReady(f.owner, current.ID, next.Revision)
	_, afterReadyRules := f.handler.ChangeMatchRules(f.owner, current.ID, ready.Revision, f.rulesInput())
	_, afterReadyChoice := f.handler.ChangeSeatChoice(f.owner, current.ID, ready.Revision, 1, f.npcAvatar, f.npcDeck)
	_, readyAgain := f.handler.DeclareReady(f.owner, current.ID, ready.Revision)

	require.ErrorIs(t, notDecided, command.ErrRoomConditionsNotDecided)
	require.NoError(t, err)
	require.Equal(t, next.Revision+1, ready.Revision)
	require.Equal(t, 9000, f.load(t, current.ID).MatchRules.TargetMagic)
	require.Len(t, f.load(t, current.ID).Ready, 1)
	require.ErrorIs(t, afterReadyRules, command.ErrRoomAlreadyReady)
	require.ErrorIs(t, afterReadyChoice, command.ErrRoomAlreadyReady)
	require.ErrorIs(t, readyAgain, command.ErrRoomAlreadyReady)
}

func TestRoomDeclareReady_RejectsBeforePublish(t *testing.T) {
	f := newRoomFixture(t)
	current := f.fullyDecided(t)

	_, err := f.handler.DeclareReady(f.owner, current.ID, current.Revision)

	require.ErrorIs(t, err, command.ErrRoomStatusNotAllowed)
}

// 選んだ時に確かめ、準備完了の時には確かめ直さない（選んだ後に使えなくなっても、対戦は始めた時点の写しで進む）
func TestRoomDeclareReady_DoesNotCheckSelectableAgain(t *testing.T) {
	f := newRoomFixture(t)
	current := f.fullyDecided(t)
	published, err := f.handler.Publish(f.owner, current.ID, current.Revision)
	require.NoError(t, err)
	f.selectable.avatars = map[uuid.UUID]bool{}
	f.selectable.npcAvatars = map[uuid.UUID]bool{}
	f.selectable.boards = map[uuid.UUID]bool{}
	asked := len(f.selectable.askedFor)

	_, err = f.handler.DeclareReady(f.owner, current.ID, published.Revision)

	require.NoError(t, err)
	require.Len(t, f.selectable.askedFor, asked)
}

func TestRoomCreate_ReturnsInputErrorForInvalidNameAndAllowsSameName(t *testing.T) {
	f := newRoomFixture(t)

	_, blank := f.handler.Create(f.owner, "  ")
	_, long := f.handler.Create(f.owner, "あいうえおかきくけこさしすせそたちつてとな")
	first, firstErr := f.handler.Create(f.owner, "同じ名前")
	second, secondErr := f.handler.Create(f.owner, "同じ名前")

	require.Equal(t, input.Errors{&input.Required{Field: "name"}}, blank)
	require.Equal(t, input.Errors{&input.TooLong{Field: "name", Max: 20}}, long)
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	require.NotEqual(t, first.ID, second.ID)
	require.Equal(t, 2, countEventsOf(t, f.ctx, roomTypeID))
}

func TestRoomRename_ChangesNameTrimmedAndKeepsRevisionForSameName(t *testing.T) {
	f := newRoomFixture(t)
	current := f.fullyDecided(t)

	renamed, err := f.handler.Rename(f.owner, current.ID, current.Revision, "  新しい名前 ")
	same, sameErr := f.handler.Rename(f.owner, current.ID, renamed.Revision, "新しい名前")

	require.NoError(t, err)
	require.NoError(t, sameErr)
	require.Equal(t, current.Revision+1, renamed.Revision)
	require.Equal(t, renamed, same)
	require.Equal(t, "新しい名前", f.load(t, current.ID).Name.String())
}

func TestRoomRename_ReturnsInputErrorsAndRejectsClosedRoom(t *testing.T) {
	f := newRoomFixture(t)
	current := f.withSeats(t)

	_, blank := f.handler.Rename(f.owner, current.ID, current.Revision, "")
	_, long := f.handler.Rename(f.owner, current.ID, current.Revision, "あいうえおかきくけこさしすせそたちつてとな")
	closed, err := f.handler.Close(f.owner, current.ID, current.Revision)
	require.NoError(t, err)
	_, onClosed := f.handler.Rename(f.owner, current.ID, closed.Revision, "新しい名前")

	require.Equal(t, input.Errors{&input.Required{Field: "name"}}, blank)
	require.Equal(t, input.Errors{&input.TooLong{Field: "name", Max: 20}}, long)
	require.ErrorIs(t, onClosed, command.ErrRoomStatusNotAllowed)
}

func TestRoomClose_ClosesAndIgnoresSecondClose(t *testing.T) {
	f := newRoomFixture(t)
	current := f.fullyDecided(t)

	closed, err := f.handler.Close(f.owner, current.ID, current.Revision)
	again, againErr := f.handler.Close(f.owner, current.ID, closed.Revision)
	_, publish := f.handler.Publish(f.owner, current.ID, closed.Revision)

	require.NoError(t, err)
	require.NoError(t, againErr)
	require.Equal(t, current.Revision+1, closed.Revision)
	require.Equal(t, closed, again)
	require.Equal(t, room.StatusClosed, f.load(t, current.ID).Status)
	require.ErrorIs(t, publish, command.ErrRoomStatusNotAllowed)
}

func gamerulestestMatch(t *testing.T, minPlayers, maxPlayers int) gamerules.Match {
	t.Helper()
	minValue, maxValue := roomtest.PlayersRange(t, minPlayers, maxPlayers)
	hand := gamerules.Defaults().Match.MaxHandCards
	return gamerules.Match{MinPlayers: minValue, MaxPlayers: maxValue, MaxHandCards: hand}
}
