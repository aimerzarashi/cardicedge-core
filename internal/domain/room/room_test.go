package room_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match/matchtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room/roomtest"
)

var (
	createdAt = time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	changedAt = time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
)

func owner(t *testing.T) player.ID {
	t.Helper()
	return playertest.ID(t, 10)
}

func guest(t *testing.T) player.ID {
	t.Helper()
	return playertest.ID(t, 11)
}

// 最後の準備完了で始まる対戦の ID（呼ぶ側が取って渡す）
func matchID(t *testing.T) match.ID {
	t.Helper()
	return matchtest.ID(t, 40)
}

func matchRules(t *testing.T) room.MatchRules {
	t.Helper()
	return roomtest.MatchRules(t, boardtest.ID(t, 5))
}

// 席の並びは プレイヤー → NPC
func seatKinds(t *testing.T) room.SeatKinds {
	t.Helper()
	return roomtest.SeatKinds(t, room.SeatKindPlayer, room.SeatKindNPC)
}

func apply(t *testing.T, state room.State, cmd room.Command) (room.State, []room.Event) {
	t.Helper()
	events, err := room.Decide(state, cmd, changedAt)
	require.NoError(t, err)
	return evolve(t, state, events...), events
}

func evolve(t *testing.T, state room.State, events ...room.Event) room.State {
	t.Helper()
	for _, event := range events {
		var err error
		state, err = room.Evolve(state, event)
		require.NoError(t, err)
	}
	return state
}

func created(t *testing.T) room.State {
	t.Helper()
	events, err := room.Decide(room.InitialState(), &room.CreateCommand{ID: roomtest.ID(t, 30), Owner: owner(t), Name: roomtest.Name(t, "はじめての部屋"), Avatars: 1, Decks: 1}, createdAt)
	require.NoError(t, err)
	return evolve(t, room.InitialState(), events...)
}

func withSeats(t *testing.T) room.State {
	t.Helper()
	state, _ := apply(t, created(t), &room.ChangeSeatsCommand{ChangedBy: owner(t), Seats: seatKinds(t)})
	return state
}

func playerChoice(t *testing.T) room.SeatChoice {
	t.Helper()
	return roomtest.PlayerChoice(t, 1, 2)
}

func npcChoice(t *testing.T) room.SeatChoice {
	t.Helper()
	return roomtest.NPCChoice(t, 3, 4)
}

func choose(t *testing.T, state room.State, seat int, choice room.SeatChoice) room.State {
	t.Helper()
	next, _ := apply(t, state, &room.ChangeSeatChoiceCommand{ChangedBy: owner(t), Seat: roomtest.SeatIndex(t, seat), Choice: choice})
	return next
}

// 席・対戦ルール・席の選択のすべてを公開前に決めた部屋
func fullyDecided(t *testing.T) room.State {
	t.Helper()
	state := withSeats(t)
	state, _ = apply(t, state, &room.ChangeMatchRulesCommand{ChangedBy: owner(t), MatchRules: matchRules(t)})
	state = choose(t, state, 0, playerChoice(t))
	return choose(t, state, 1, npcChoice(t))
}

func publish(t *testing.T, state room.State) room.State {
	t.Helper()
	next, _ := apply(t, state, &room.PublishCommand{PublishedBy: owner(t)})
	return next
}

// 席の並びだけを決めて公開した部屋（対戦ルールと席の選択は準備中に決める）
func preparing(t *testing.T) room.State {
	t.Helper()
	return publish(t, withSeats(t))
}

func TestDecide_CreateEmitsCreatedEventAndStartsDraftWithoutSeats(t *testing.T) {
	cmd := &room.CreateCommand{ID: roomtest.ID(t, 30), Owner: owner(t), Name: roomtest.Name(t, "はじめての部屋"), Avatars: 1, Decks: 1}

	events, err := room.Decide(room.InitialState(), cmd, createdAt)
	require.NoError(t, err)
	state := evolve(t, room.InitialState(), events...)

	require.Equal(t, []room.Event{&room.CreatedEvent{
		EventMeta: room.EventMeta{EntityID: cmd.ID, AggregateVersion: 1, OccurredAt: createdAt},
		Player:    owner(t),
		Name:      cmd.Name,
	}}, events)
	require.Equal(t, room.Room{ID: cmd.ID, Version: 1, Owner: owner(t), Name: cmd.Name, Status: room.StatusDraft}, state)
}

func TestDecide_RejectsCreateWhenAlreadyCreated(t *testing.T) {
	_, err := room.Decide(created(t), &room.CreateCommand{ID: roomtest.ID(t, 30), Owner: owner(t), Name: roomtest.Name(t, "ふたつめ"), Avatars: 1, Decks: 1}, createdAt)

	require.ErrorIs(t, err, room.ErrAlreadyCreated)
}

func TestDecide_RejectsEveryOtherCommandBeforeCreated(t *testing.T) {
	_, err := room.Decide(room.InitialState(), &room.PublishCommand{PublishedBy: owner(t)}, changedAt)

	require.ErrorIs(t, err, room.ErrInvalidState)
}

func TestNewSeatKinds_ChecksSeatCountThenPlayerSeatCount(t *testing.T) {
	player, npc := room.SeatKindPlayer, room.SeatKindNPC
	tests := []struct {
		name  string
		kinds []room.SeatKind
		want  error
	}{
		{name: "player_and_npc", kinds: []room.SeatKind{player, npc}},
		{name: "four_seats", kinds: []room.SeatKind{player, npc, npc, npc}},
		{name: "too_few_seats", kinds: []room.SeatKind{player}, want: room.ErrTooFewSeats},
		{name: "too_many_seats", kinds: []room.SeatKind{player, npc, npc, npc, npc}, want: room.ErrTooManySeats},
		{name: "no_player_seat", kinds: []room.SeatKind{npc, npc}},
		{name: "two_player_seats", kinds: []room.SeatKind{player, player}, want: room.ErrTooManyPlayerSeats},
		// 席の数が範囲の外なら、プレイヤーの席の数は確かめない
		{name: "seat_count_is_checked_first", kinds: []room.SeatKind{npc, npc, npc, npc, npc}, want: room.ErrTooManySeats},
	}
	minPlayers, maxPlayers := roomtest.PlayersRange(t, 2, 4)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seats, err := room.NewSeatKinds(tt.kinds, minPlayers, maxPlayers)

			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.kinds, []room.SeatKind(seats))
		})
	}
}

func TestNewSeatKinds_UsesGivenPlayersRange(t *testing.T) {
	minPlayers, maxPlayers := roomtest.PlayersRange(t, 3, 3)

	_, tooFew := room.NewSeatKinds([]room.SeatKind{room.SeatKindPlayer, room.SeatKindNPC}, minPlayers, maxPlayers)

	require.ErrorIs(t, tooFew, room.ErrTooFewSeats)
}

func TestNewSeatKind_RejectsUnknownKind(t *testing.T) {
	_, err := room.NewSeatKind("team")

	require.ErrorIs(t, err, room.ErrSeatKindUnknown)
}

func TestNewSeatIndex_RejectsNegative(t *testing.T) {
	_, err := room.NewSeatIndex(-1)

	require.ErrorIs(t, err, room.ErrSeatIndexNegative)
}

func TestNewMatchRules_ReturnsBoardDefaultRulesViolations(t *testing.T) {
	_, err := room.NewMatchRules(boardtest.ID(t, 5), 0, 0, 0, 0, 1, 1, 6, 100, 0)

	require.ErrorIs(t, err, board.ErrTargetMagicTooSmall)
	require.ErrorIs(t, err, board.ErrTargetMagicNotGreaterThanStartingMagic)
}

func TestDecide_ChangeSeatsSeatsOwnerInPlayerSeat(t *testing.T) {
	state, events := apply(t, created(t), &room.ChangeSeatsCommand{ChangedBy: owner(t), Seats: seatKinds(t)})

	require.Equal(t, []room.Event{&room.SeatsChangedEvent{
		EventMeta: room.EventMeta{EntityID: roomtest.ID(t, 30), AggregateVersion: 2, OccurredAt: changedAt},
		Seats:     []room.Seat{{Kind: room.SeatKindPlayer, Player: owner(t)}, {Kind: room.SeatKindNPC}},
	}}, events)
	playerSeat := room.Seat{Kind: room.SeatKindPlayer, Player: owner(t)}
	npcSeat := room.Seat{Kind: room.SeatKindNPC}
	require.Equal(t, []room.SeatState{{Seat: playerSeat}, {Seat: npcSeat}}, state.(room.Room).Seats)
}

func TestDecide_ChangeSeatsClearsEverySeatChoice(t *testing.T) {
	state := choose(t, choose(t, withSeats(t), 0, playerChoice(t)), 1, npcChoice(t))

	next, _ := apply(t, state, &room.ChangeSeatsCommand{ChangedBy: owner(t), Seats: roomtest.SeatKinds(t, room.SeatKindNPC, room.SeatKindPlayer)})

	for _, seat := range next.(room.Room).Seats {
		require.Nil(t, seat.Choice)
	}
}

func TestDecide_ChangeSeatsEmitsNothingForSameSeats(t *testing.T) {
	state := choose(t, withSeats(t), 0, playerChoice(t))

	next, events := apply(t, state, &room.ChangeSeatsCommand{ChangedBy: owner(t), Seats: seatKinds(t)})

	require.Empty(t, events)
	require.NotNil(t, next.(room.Room).Seats[0].Choice)
}

func TestDecide_ChangeSeatsRejectsAnyoneButOwner(t *testing.T) {
	_, err := room.Decide(created(t), &room.ChangeSeatsCommand{ChangedBy: guest(t), Seats: seatKinds(t)}, changedAt)

	require.ErrorIs(t, err, room.ErrNotOwner)
}

func TestDecide_ChangeSeatsIsAllowedOnlyInDraft(t *testing.T) {
	closed, _ := apply(t, created(t), &room.CloseCommand{ClosedBy: owner(t)})
	tests := []struct {
		name  string
		state room.State
	}{
		{name: "preparing", state: preparing(t)},
		{name: "closed", state: closed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := room.Decide(tt.state, &room.ChangeSeatsCommand{ChangedBy: owner(t), Seats: roomtest.SeatKinds(t, room.SeatKindPlayer, room.SeatKindNPC, room.SeatKindNPC)}, changedAt)

			require.ErrorIs(t, err, room.ErrStatusNotAllowed)
		})
	}
}

func TestDecide_ChangeMatchRulesDecidesInDraftAndEmitsNothingForSameValue(t *testing.T) {
	state, events := apply(t, created(t), &room.ChangeMatchRulesCommand{ChangedBy: owner(t), MatchRules: matchRules(t)})
	_, again := apply(t, state, &room.ChangeMatchRulesCommand{ChangedBy: owner(t), MatchRules: matchRules(t)})

	require.Equal(t, []room.Event{&room.MatchRulesChangedEvent{
		EventMeta:  room.EventMeta{EntityID: roomtest.ID(t, 30), AggregateVersion: 2, OccurredAt: changedAt},
		MatchRules: matchRules(t),
	}}, events)
	require.Equal(t, matchRules(t), *state.(room.Room).MatchRules)
	require.Empty(t, again)
}

func TestDecide_ChangeMatchRulesAllowsOwnerToDecideInPreparingWhenNotDecidedBeforePublish(t *testing.T) {
	state := preparing(t)

	decided, _ := apply(t, state, &room.ChangeMatchRulesCommand{ChangedBy: owner(t), MatchRules: matchRules(t)})
	// 公開前に決めていないので、準備完了までは選び直せる
	other := roomtest.MatchRules(t, boardtest.ID(t, 6))
	rechosen, _ := apply(t, decided, &room.ChangeMatchRulesCommand{ChangedBy: owner(t), MatchRules: other})

	require.Equal(t, other, *rechosen.(room.Room).MatchRules)
}

func TestDecide_ChangeMatchRulesRejectsChangeInPreparingWhenDecidedBeforePublish(t *testing.T) {
	state := publish(t, fullyDecided(t))

	_, err := room.Decide(state, &room.ChangeMatchRulesCommand{ChangedBy: owner(t), MatchRules: roomtest.MatchRules(t, boardtest.ID(t, 6))}, changedAt)

	require.ErrorIs(t, err, room.ErrConditionLocked)
}

func TestDecide_ChangeMatchRulesRejectsNonOwner(t *testing.T) {
	tests := []struct {
		name  string
		state room.State
		want  error
	}{
		{name: "draft", state: created(t), want: room.ErrNotOwner},
		{name: "preparing", state: preparing(t), want: room.ErrNotResponsible},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := room.Decide(tt.state, &room.ChangeMatchRulesCommand{ChangedBy: guest(t), MatchRules: matchRules(t)}, changedAt)

			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestDecide_ChangeSeatChoiceDecidesAndRechoosesPerSeat(t *testing.T) {
	state := withSeats(t)

	state, events := apply(t, state, &room.ChangeSeatChoiceCommand{ChangedBy: owner(t), Seat: roomtest.SeatIndex(t, 1), Choice: npcChoice(t)})
	_, same := apply(t, state, &room.ChangeSeatChoiceCommand{ChangedBy: owner(t), Seat: roomtest.SeatIndex(t, 1), Choice: npcChoice(t)})
	rechosen := roomtest.NPCChoice(t, 3, 9)
	state = choose(t, state, 1, rechosen)

	require.Equal(t, []room.Event{&room.SeatChoiceChangedEvent{
		EventMeta: room.EventMeta{EntityID: roomtest.ID(t, 30), AggregateVersion: 3, OccurredAt: changedAt},
		Seat:      roomtest.SeatIndex(t, 1), Choice: npcChoice(t), ChangedBy: owner(t),
	}}, events)
	require.Empty(t, same)
	require.Equal(t, rechosen, *state.(room.Room).Seats[1].Choice)
	require.Nil(t, state.(room.Room).Seats[0].Choice)
}

func TestDecide_ChangeSeatChoiceRejectsInvalidSeats(t *testing.T) {
	tests := []struct {
		name   string
		state  room.State
		seat   int
		choice room.SeatChoice
		want   error
	}{
		{name: "seats_not_decided", state: created(t), seat: 0, choice: playerChoice(t), want: room.ErrSeatsNotDecided},
		{name: "seat_out_of_range", state: withSeats(t), seat: 2, choice: playerChoice(t), want: room.ErrSeatNotFound},
		{name: "npc_choice_for_player_seat", state: withSeats(t), seat: 0, choice: npcChoice(t), want: room.ErrSeatKindMismatch},
		{name: "player_choice_for_npc_seat", state: withSeats(t), seat: 1, choice: playerChoice(t), want: room.ErrSeatKindMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := room.Decide(tt.state, &room.ChangeSeatChoiceCommand{ChangedBy: owner(t), Seat: roomtest.SeatIndex(t, tt.seat), Choice: tt.choice}, changedAt)

			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestDecide_ChangeSeatChoiceKeepsChoiceDecidedBeforePublishButLetsPreparingDecideTheRest(t *testing.T) {
	// プレイヤーの席だけ公開前に決める。NPC の席は準備中に決める
	state := publish(t, choose(t, withSeats(t), 0, playerChoice(t)))

	_, locked := room.Decide(state, &room.ChangeSeatChoiceCommand{ChangedBy: owner(t), Seat: roomtest.SeatIndex(t, 0), Choice: roomtest.PlayerChoice(t, 5, 6)}, changedAt)
	decided := choose(t, state, 1, npcChoice(t))
	rechosen := choose(t, decided, 1, roomtest.NPCChoice(t, 3, 9))

	require.ErrorIs(t, locked, room.ErrConditionLocked)
	require.Equal(t, roomtest.NPCChoice(t, 3, 9), *rechosen.(room.Room).Seats[1].Choice)
}

// 作った人だけの部屋では準備完了ですぐ対戦が始まるので、ゲストの席がある部屋で、ゲストがまだのうちに作った人が準備完了する
func TestDecide_ChangeSeatChoiceRejectsAfterReadyOfResponsiblePerson(t *testing.T) {
	state := preparingWithGuestSeat(t)
	state, _ = apply(t, state, &room.ChangeMatchRulesCommand{ChangedBy: owner(t), MatchRules: matchRules(t)})
	state = choose(t, state, 1, npcChoice(t))
	state, _ = apply(t, state, &room.DeclareReadyCommand{Player: owner(t)})

	_, err := room.Decide(state, &room.ChangeSeatChoiceCommand{ChangedBy: owner(t), Seat: roomtest.SeatIndex(t, 1), Choice: roomtest.NPCChoice(t, 3, 9)}, changedAt)
	_, rules := room.Decide(state, &room.ChangeMatchRulesCommand{ChangedBy: owner(t), MatchRules: roomtest.MatchRules(t, boardtest.ID(t, 6))}, changedAt)

	require.ErrorIs(t, err, room.ErrAlreadyReady)
	require.ErrorIs(t, rules, room.ErrAlreadyReady)
}

func TestDecide_ChangeSeatChoiceRejectsNonOwnerInDraft(t *testing.T) {
	_, err := room.Decide(withSeats(t), &room.ChangeSeatChoiceCommand{ChangedBy: guest(t), Seat: roomtest.SeatIndex(t, 0), Choice: playerChoice(t)}, changedAt)

	require.ErrorIs(t, err, room.ErrNotOwner)
}

func TestDecide_ChangeSeatChoiceRejectsClosedRoom(t *testing.T) {
	closed, _ := apply(t, withSeats(t), &room.CloseCommand{ClosedBy: owner(t)})

	_, err := room.Decide(closed, &room.ChangeSeatChoiceCommand{ChangedBy: owner(t), Seat: roomtest.SeatIndex(t, 0), Choice: playerChoice(t)}, changedAt)

	require.ErrorIs(t, err, room.ErrStatusNotAllowed)
}

func TestDecide_PublishEmitsPublishedThenPreparationStartedAndLocksDecidedConditions(t *testing.T) {
	state := choose(t, withSeats(t), 0, playerChoice(t))

	published, events := apply(t, state, &room.PublishCommand{PublishedBy: owner(t)})

	require.Equal(t, []room.Event{
		&room.PublishedEvent{EventMeta: room.EventMeta{EntityID: roomtest.ID(t, 30), AggregateVersion: 4, OccurredAt: changedAt}},
		&room.PreparationStartedEvent{EventMeta: room.EventMeta{EntityID: roomtest.ID(t, 30), AggregateVersion: 5, OccurredAt: changedAt}},
	}, events)
	current := published.(room.Room)
	require.Equal(t, room.StatusPreparing, current.Status)
	require.Equal(t, int64(5), current.Version)
	require.True(t, current.Seats[0].ChoiceLocked)
	require.False(t, current.Seats[1].ChoiceLocked)
	require.False(t, current.MatchRulesLocked)
}

func TestDecide_PublishRejectsUndecidedSeatsNonOwnerAndNotDraft(t *testing.T) {
	closed, _ := apply(t, withSeats(t), &room.CloseCommand{ClosedBy: owner(t)})
	tests := []struct {
		name        string
		state       room.State
		publishedBy player.ID
		want        error
	}{
		{name: "seats_not_decided", state: created(t), publishedBy: owner(t), want: room.ErrSeatsNotDecided},
		{name: "not_owner", state: withSeats(t), publishedBy: guest(t), want: room.ErrNotOwner},
		{name: "already_published", state: preparing(t), publishedBy: owner(t), want: room.ErrStatusNotAllowed},
		{name: "closed", state: closed, publishedBy: owner(t), want: room.ErrStatusNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := room.Decide(tt.state, &room.PublishCommand{PublishedBy: tt.publishedBy}, changedAt)

			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestDecide_PublishDoesNotRequireMatchRulesOrChoices(t *testing.T) {
	_, events := apply(t, withSeats(t), &room.PublishCommand{PublishedBy: owner(t)})

	require.Len(t, events, 2)
}

// 部屋に入っているのが作った人だけなら、その準備完了が最後で、続けて対戦が始まる
func TestDecide_DeclareReadyRecordsReadyAndStartsMatchWhenEveryoneIsReady(t *testing.T) {
	state := publish(t, fullyDecided(t))

	ready, events := apply(t, state, &room.DeclareReadyCommand{Player: owner(t), Match: matchID(t)})

	require.Equal(t, []room.Event{
		&room.ReadyDeclaredEvent{EventMeta: room.EventMeta{EntityID: roomtest.ID(t, 30), AggregateVersion: 8, OccurredAt: changedAt}, Player: owner(t)},
		&room.MatchStartedEvent{EventMeta: room.EventMeta{EntityID: roomtest.ID(t, 30), AggregateVersion: 9, OccurredAt: changedAt}, Match: matchID(t)},
	}, events)
	current := ready.(room.Room)
	require.Equal(t, []player.ID{owner(t)}, current.Ready)
	require.Equal(t, room.StatusPlaying, current.Status)
	require.Equal(t, matchID(t), current.Match)
}

// ほかの人がまだなら対戦は始まらず、対戦の ID も要らない（渡されても使わない）
func TestDecide_DeclareReadyDoesNotStartMatchWhileSomeoneIsNotReady(t *testing.T) {
	state := evolveChoice(t, preparingWithGuestSeat(t), guest(t), 0, playerChoice(t))
	require.False(t, state.(room.Room).StartsMatchOnReady(guest(t)))

	ready, events := apply(t, state, &room.DeclareReadyCommand{Player: guest(t)})

	require.Len(t, events, 1)
	require.Equal(t, room.StatusPreparing, ready.(room.Room).Status)
	require.Zero(t, ready.(room.Room).Match)
}

func TestDecide_DeclareReadyRejectsMissingMatchIDWhenItStartsTheMatch(t *testing.T) {
	state := publish(t, fullyDecided(t))
	require.True(t, state.(room.Room).StartsMatchOnReady(owner(t)))

	_, err := room.Decide(state, &room.DeclareReadyCommand{Player: owner(t)}, changedAt)

	require.ErrorIs(t, err, room.ErrMatchIDMissing)
}

func TestRoom_StartsMatchOnReadyIsFalseForThoseWhoCannotDeclareReady(t *testing.T) {
	state := publish(t, fullyDecided(t)).(room.Room)

	require.False(t, state.StartsMatchOnReady(guest(t)))
	require.False(t, preparing(t).(room.Room).StartsMatchOnReady(owner(t)))
}

func TestDecide_PlayingRoomCannotBeClosedOrChangedButCanBeRenamed(t *testing.T) {
	playing, _ := apply(t, publish(t, fullyDecided(t)), &room.DeclareReadyCommand{Player: owner(t), Match: matchID(t)})

	_, closeErr := room.Decide(playing, &room.CloseCommand{ClosedBy: owner(t)}, changedAt)
	_, rulesErr := room.Decide(playing, &room.ChangeMatchRulesCommand{ChangedBy: owner(t), MatchRules: matchRules(t)}, changedAt)
	_, renameErr := room.Decide(playing, &room.RenameCommand{RenamedBy: owner(t), Name: roomtest.Name(t, "対戦中の部屋")}, changedAt)

	require.ErrorIs(t, closeErr, room.ErrStatusNotAllowed)
	require.ErrorIs(t, rulesErr, room.ErrStatusNotAllowed)
	require.NoError(t, renameErr)
	require.Equal(t, room.Capabilities{ChangeSeatChoice: []bool{false, false}, Rename: true}, room.CapabilitiesOf(playing, owner(t)))
}

func TestDecide_DeclareReadyRejectsWhenSomethingOfThePersonIsNotDecided(t *testing.T) {
	player0 := func(state room.State) room.State { return choose(t, state, 0, playerChoice(t)) }
	npc1 := func(state room.State) room.State { return choose(t, state, 1, npcChoice(t)) }
	rules := func(state room.State) room.State {
		next, _ := apply(t, state, &room.ChangeMatchRulesCommand{ChangedBy: owner(t), MatchRules: matchRules(t)})
		return next
	}
	tests := []struct {
		name  string
		state room.State
	}{
		{name: "nothing_decided", state: preparing(t)},
		{name: "match_rules_missing", state: npc1(player0(preparing(t)))},
		{name: "npc_seat_missing", state: rules(player0(preparing(t)))},
		{name: "player_seat_missing", state: rules(npc1(preparing(t)))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := room.Decide(tt.state, &room.DeclareReadyCommand{Player: owner(t), Match: matchID(t)}, changedAt)

			require.ErrorIs(t, err, room.ErrConditionsNotDecided)
		})
	}
}

func TestDecide_DeclareReadyRejectsOutsideTheRoomAndStatesAndRepeats(t *testing.T) {
	ready, _ := apply(t, evolveChoice(t, preparingWithGuestSeat(t), guest(t), 0, playerChoice(t)), &room.DeclareReadyCommand{Player: guest(t)})
	tests := []struct {
		name   string
		state  room.State
		player player.ID
		want   error
	}{
		{name: "draft", state: fullyDecided(t), player: owner(t), want: room.ErrStatusNotAllowed},
		{name: "not_participant", state: publish(t, fullyDecided(t)), player: guest(t), want: room.ErrNotParticipant},
		{name: "already_ready", state: ready, player: guest(t), want: room.ErrAlreadyReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := room.Decide(tt.state, &room.DeclareReadyCommand{Player: tt.player, Match: matchID(t)}, changedAt)

			require.ErrorIs(t, err, tt.want)
		})
	}
}

// ほかの人がプレイヤーの席に座る部屋（このマイルストーンは作った人だけだが、担当の決まりは席に座っている人で分ける）を、イベントで作る。
// 席は プレイヤー（guest）→ プレイヤーではない NPC。作った人は NPC の席と対戦ルールの担当
func preparingWithGuestSeat(t *testing.T) room.State {
	t.Helper()
	meta := func(version int64) room.EventMeta {
		return room.EventMeta{EntityID: roomtest.ID(t, 31), AggregateVersion: version, OccurredAt: createdAt}
	}
	return evolve(t, room.InitialState(),
		&room.CreatedEvent{EventMeta: meta(1), Player: owner(t), Name: roomtest.Name(t, "ゲストの席つき")},
		&room.SeatsChangedEvent{EventMeta: meta(2), Seats: []room.Seat{{Kind: room.SeatKindPlayer, Player: guest(t)}, {Kind: room.SeatKindNPC}}},
		&room.PublishedEvent{EventMeta: meta(3)},
		&room.PreparationStartedEvent{EventMeta: meta(4)},
	)
}

func TestDecide_ChangeSeatChoiceInPreparingIsLeftToTheSeatsPlayerOrTheOwnerForNPCSeats(t *testing.T) {
	state := preparingWithGuestSeat(t)

	_, ownerOnPlayerSeat := room.Decide(state, &room.ChangeSeatChoiceCommand{ChangedBy: owner(t), Seat: roomtest.SeatIndex(t, 0), Choice: playerChoice(t)}, changedAt)
	_, guestOnNPCSeat := room.Decide(state, &room.ChangeSeatChoiceCommand{ChangedBy: guest(t), Seat: roomtest.SeatIndex(t, 1), Choice: npcChoice(t)}, changedAt)
	_, guestOnPlayerSeat := room.Decide(state, &room.ChangeSeatChoiceCommand{ChangedBy: guest(t), Seat: roomtest.SeatIndex(t, 0), Choice: playerChoice(t)}, changedAt)
	_, ownerOnNPCSeat := room.Decide(state, &room.ChangeSeatChoiceCommand{ChangedBy: owner(t), Seat: roomtest.SeatIndex(t, 1), Choice: npcChoice(t)}, changedAt)

	require.ErrorIs(t, ownerOnPlayerSeat, room.ErrNotResponsible)
	require.ErrorIs(t, guestOnNPCSeat, room.ErrNotResponsible)
	require.NoError(t, guestOnPlayerSeat)
	require.NoError(t, ownerOnNPCSeat)
}

// 準備完了はほかの人の選び直しでは外れず、自分の担当の分だけで押せる
func TestDecide_DeclareReadyIsKeptWhenAnotherPersonChangesTheirOwnConditions(t *testing.T) {
	state := preparingWithGuestSeat(t)
	state = evolveChoice(t, state, guest(t), 0, playerChoice(t))
	state, _ = apply(t, state, &room.DeclareReadyCommand{Player: guest(t), Match: matchID(t)})

	state, _ = apply(t, state, &room.ChangeMatchRulesCommand{ChangedBy: owner(t), MatchRules: matchRules(t)})
	state = evolveChoice(t, state, owner(t), 1, npcChoice(t))
	_, ownerNotReady := room.Decide(state, &room.DeclareReadyCommand{Player: guest(t), Match: matchID(t)}, changedAt)
	state, _ = apply(t, state, &room.DeclareReadyCommand{Player: owner(t), Match: matchID(t)})

	require.ErrorIs(t, ownerNotReady, room.ErrAlreadyReady)
	require.ElementsMatch(t, []player.ID{guest(t), owner(t)}, state.(room.Room).Ready)
}

func evolveChoice(t *testing.T, state room.State, by player.ID, seat int, choice room.SeatChoice) room.State {
	t.Helper()
	next, _ := apply(t, state, &room.ChangeSeatChoiceCommand{ChangedBy: by, Seat: roomtest.SeatIndex(t, seat), Choice: choice})
	return next
}

func TestDecide_CloseClosesDraftAndPreparingRoomsAndIgnoresClosedOne(t *testing.T) {
	tests := []struct {
		name  string
		state room.State
	}{
		{name: "draft", state: created(t)},
		{name: "preparing", state: preparing(t)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			closed, events := apply(t, tt.state, &room.CloseCommand{ClosedBy: owner(t)})
			_, again := apply(t, closed, &room.CloseCommand{ClosedBy: owner(t)})

			require.Len(t, events, 1)
			require.IsType(t, &room.ClosedEvent{}, events[0])
			require.Equal(t, room.StatusClosed, closed.(room.Room).Status)
			require.Empty(t, again)
		})
	}
}

func TestDecide_CloseRejectsAnyoneButOwner(t *testing.T) {
	_, err := room.Decide(created(t), &room.CloseCommand{ClosedBy: guest(t)}, changedAt)

	require.ErrorIs(t, err, room.ErrNotOwner)
}

func TestDecide_RejectsEveryOperationOnClosedRoom(t *testing.T) {
	closed, _ := apply(t, fullyDecided(t), &room.CloseCommand{ClosedBy: owner(t)})
	commands := []room.Command{
		&room.ChangeSeatsCommand{ChangedBy: owner(t), Seats: seatKinds(t)},
		&room.ChangeMatchRulesCommand{ChangedBy: owner(t), MatchRules: roomtest.MatchRules(t, boardtest.ID(t, 6))},
		&room.PublishCommand{PublishedBy: owner(t)},
		&room.DeclareReadyCommand{Player: owner(t), Match: matchID(t)},
		&room.RenameCommand{RenamedBy: owner(t), Name: roomtest.Name(t, "新しい名前")},
	}
	for _, cmd := range commands {
		_, err := room.Decide(closed, cmd, changedAt)

		require.ErrorIs(t, err, room.ErrStatusNotAllowed)
	}
}

func TestEvolve_DoesNotChangeGivenState(t *testing.T) {
	state := withSeats(t)

	choose(t, state, 0, playerChoice(t))
	publish(t, state)

	require.Nil(t, state.(room.Room).Seats[0].Choice)
	require.False(t, state.(room.Room).Seats[0].ChoiceLocked)
	require.Equal(t, room.StatusDraft, state.(room.Room).Status)
}

func TestEvolve_RejectsEventsBeforeCreatedAndSecondCreated(t *testing.T) {
	_, before := room.Evolve(room.InitialState(), &room.ClosedEvent{})
	_, twice := room.Evolve(created(t), &room.CreatedEvent{Player: owner(t)})
	_, outOfRange := room.Evolve(withSeats(t), &room.SeatChoiceChangedEvent{Seat: roomtest.SeatIndex(t, 5), Choice: playerChoice(t)})

	require.ErrorIs(t, before, room.ErrInvalidState)
	require.ErrorIs(t, twice, room.ErrInvalidState)
	require.ErrorIs(t, outOfRange, room.ErrInvalidState)
}

func TestRoom_HasParticipantIsOwnerAndSeatedPlayers(t *testing.T) {
	state := preparingWithGuestSeat(t).(room.Room)

	require.True(t, state.HasParticipant(owner(t)))
	require.True(t, state.HasParticipant(guest(t)))
	require.False(t, state.HasParticipant(playertest.ID(t, 12)))
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	meta := room.EventMeta{EntityID: roomtest.ID(t, 30), AggregateVersion: 2, OccurredAt: changedAt}
	events := []room.Event{
		&room.CreatedEvent{EventMeta: meta, Player: owner(t), Name: roomtest.Name(t, "はじめての部屋")},
		&room.SeatsChangedEvent{EventMeta: meta, Seats: []room.Seat{{Kind: room.SeatKindPlayer, Player: owner(t)}, {Kind: room.SeatKindNPC}}},
		&room.MatchRulesChangedEvent{EventMeta: meta, MatchRules: matchRules(t)},
		&room.SeatChoiceChangedEvent{EventMeta: meta, Seat: roomtest.SeatIndex(t, 1), Choice: roomtest.NPCChoice(t, 7, 8), ChangedBy: owner(t)},
		&room.PublishedEvent{EventMeta: meta},
		&room.PreparationStartedEvent{EventMeta: meta},
		&room.ReadyDeclaredEvent{EventMeta: meta, Player: owner(t)},
		&room.MatchStartedEvent{EventMeta: meta, Match: matchID(t)},
		&room.MatchFinishedEvent{EventMeta: meta, Match: matchID(t)},
		&room.NextDeclaredEvent{EventMeta: meta, Player: owner(t), Continue: true},
		&room.PreparationRestartedEvent{EventMeta: meta},
		&room.PausedEvent{EventMeta: meta},
		&room.ClosedEvent{EventMeta: meta},
		&room.RenamedEvent{EventMeta: meta, Name: roomtest.Name(t, "新しい名前")},
	}
	for _, event := range events {
		t.Run(event.EventType().String(), func(t *testing.T) {
			require.Equal(t, 1, event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := room.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, event, restored)
		})
	}
}

func TestSeatChoiceChangedEvent_RestoresPlayerSeatChoice(t *testing.T) {
	event := &room.SeatChoiceChangedEvent{Seat: roomtest.SeatIndex(t, 0), Choice: playerChoice(t), ChangedBy: owner(t)}
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	restored, err := room.NewEventType(event.EventType()).Unmarshal(room.EventMeta{}, 1, payload)

	require.NoError(t, err)
	require.JSONEq(t, `{"seat":0,"choice":{"player":{"avatar":1,"deck":2}},"changed_by":10}`, string(payload))
	require.Equal(t, event, restored)
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := room.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(room.EventMeta{}, 1, []byte(`{}`))

	require.Error(t, err)
}

func TestNewName_TrimsAndChecksLengthByCharacters(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
		err   error
	}{
		{name: "trimmed", value: "  対戦部屋 ", want: "対戦部屋"},
		{name: "max_length_in_characters", value: "あいうえおかきくけこさしすせそたちつてと", want: "あいうえおかきくけこさしすせそたちつてと"},
		{name: "blank", value: " \u3000 ", err: room.ErrNameEmpty},
		{name: "empty", value: "", err: room.ErrNameEmpty},
		{name: "one_over_max_length", value: "あいうえおかきくけこさしすせそたちつてとな", err: room.ErrNameTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := room.NewName(tt.value)

			require.ErrorIs(t, err, tt.err)
			require.Equal(t, tt.want, got.String())
		})
	}
}

func TestDecide_RenameChangesNameInAnyStatusButClosedAndEmitsNothingForSameName(t *testing.T) {
	newName := roomtest.Name(t, "新しい名前")
	for name, state := range map[string]room.State{"draft": created(t), "preparing": preparing(t)} {
		t.Run(name, func(t *testing.T) {
			renamed, events := apply(t, state, &room.RenameCommand{RenamedBy: owner(t), Name: newName})
			_, again := apply(t, renamed, &room.RenameCommand{RenamedBy: owner(t), Name: newName})

			require.Len(t, events, 1)
			require.Equal(t, &room.RenamedEvent{EventMeta: room.EventMeta{EntityID: roomtest.ID(t, 30), AggregateVersion: state.(room.Room).Version + 1, OccurredAt: changedAt}, Name: newName}, events[0])
			require.Equal(t, newName, renamed.(room.Room).Name)
			require.Empty(t, again)
		})
	}
}

func TestDecide_RenameRejectsAnyoneButOwnerAndClosedRoom(t *testing.T) {
	closed, _ := apply(t, created(t), &room.CloseCommand{ClosedBy: owner(t)})

	_, notOwner := room.Decide(created(t), &room.RenameCommand{RenamedBy: guest(t), Name: roomtest.Name(t, "x")}, changedAt)
	_, onClosed := room.Decide(closed, &room.RenameCommand{RenamedBy: owner(t), Name: roomtest.Name(t, "x")}, changedAt)

	require.ErrorIs(t, notOwner, room.ErrNotOwner)
	require.ErrorIs(t, onClosed, room.ErrStatusNotAllowed)
}

// 見ている人にできる操作は、Decide と同じ関数で判断する。2 つがずれないことを、状態 × 見ている人 × 操作の全部の組で確かめる
func TestCapabilitiesOf_AgreesWithDecideForEveryStateViewerAndOperation(t *testing.T) {
	closed, _ := apply(t, fullyDecided(t), &room.CloseCommand{ClosedBy: owner(t)})
	deciding := decidingNext(t, 1)
	paused, _ := apply(t, deciding, &room.DeclareNextCommand{Player: guest(t), Continue: false})
	playing, _ := apply(t, evolveChoicesInPreparing(t), &room.DeclareReadyCommand{Player: owner(t), Match: matchID(t)})
	readyGuest, _ := apply(t, evolveChoice(t, preparingWithGuestSeat(t), guest(t), 0, playerChoice(t)), &room.DeclareReadyCommand{Player: guest(t)})
	states := map[string]room.State{
		"uncreated":          room.InitialState(),
		"draft_without_seat": created(t),
		"draft_with_seats":   withSeats(t),
		"draft_decided":      fullyDecided(t),
		"preparing":          preparing(t),
		"preparing_decided":  publish(t, fullyDecided(t)),
		"preparing_guest":    preparingWithGuestSeat(t),
		"preparing_ready":    readyGuest,
		"playing":            playing,
		"deciding_next":      deciding,
		"paused":             paused,
		"closed":             closed,
	}
	viewers := map[string]player.ID{"owner": owner(t), "guest": guest(t), "outsider": playertest.ID(t, 12)}
	for stateName, state := range states {
		for viewerName, viewer := range viewers {
			t.Run(stateName+"/"+viewerName, func(t *testing.T) {
				got := room.CapabilitiesOf(state, viewer)

				allowed := func(cmd room.Command) bool {
					events, err := room.Decide(state, cmd, changedAt)
					return err == nil && len(events) > 0
				}
				allowedEvenIfNothingChanges := func(cmd room.Command) bool {
					_, err := room.Decide(state, cmd, changedAt)
					return err == nil
				}
				require.Equal(t, allowedEvenIfNothingChanges(&room.ChangeSeatsCommand{ChangedBy: viewer, Seats: seatKinds(t)}), got.ChangeSeats)
				require.Equal(t, allowedEvenIfNothingChanges(&room.ChangeMatchRulesCommand{ChangedBy: viewer, MatchRules: matchRules(t)}), got.ChangeMatchRules)
				require.Equal(t, allowedEvenIfNothingChanges(&room.PublishCommand{PublishedBy: viewer}), got.Publish)
				require.Equal(t, allowedEvenIfNothingChanges(&room.DeclareReadyCommand{Player: viewer, Match: matchID(t)}), got.DeclareReady)
				require.Equal(t, allowedEvenIfNothingChanges(&room.DeclareNextCommand{Player: viewer, Continue: true}), got.DeclareNext)
				require.Equal(t, allowedEvenIfNothingChanges(&room.DeclareNextCommand{Player: viewer, Continue: false}), got.DeclareNext)
				require.Equal(t, allowedEvenIfNothingChanges(&room.ResumeCommand{ResumedBy: viewer}), got.Resume)
				require.Equal(t, allowedEvenIfNothingChanges(&room.RenameCommand{RenamedBy: viewer, Name: roomtest.Name(t, "x")}), got.Rename)
				// 閉じた部屋を閉じ直すのは成功するが何も変わらないので、できる操作には数えない
				require.Equal(t, allowed(&room.CloseCommand{ClosedBy: viewer}), got.Close)

				seats := 0
				if current, ok := state.(room.Room); ok {
					seats = len(current.Seats)
					for i, seat := range current.Seats {
						choice := playerChoice(t)
						if seat.Seat.Kind == room.SeatKindNPC {
							choice = npcChoice(t)
						}
						cmd := &room.ChangeSeatChoiceCommand{ChangedBy: viewer, Seat: roomtest.SeatIndex(t, i), Choice: choice}
						require.Equal(t, allowedEvenIfNothingChanges(cmd), got.ChangeSeatChoice[i])
					}
				}
				require.Len(t, got.ChangeSeatChoice, seats)
			})
		}
	}
}

// 公開後に、対戦ルールと席の選択をすべて決めた部屋
func evolveChoicesInPreparing(t *testing.T) room.State {
	t.Helper()
	state := publish(t, withSeats(t))
	state, _ = apply(t, state, &room.ChangeMatchRulesCommand{ChangedBy: owner(t), MatchRules: matchRules(t)})
	state = choose(t, state, 0, playerChoice(t))
	return choose(t, state, 1, npcChoice(t))
}

func TestCapabilitiesOf_ShowsWhatTheOwnerCanDoAcrossTheLifecycle(t *testing.T) {
	draft := room.CapabilitiesOf(withSeats(t), owner(t))
	prepared := room.CapabilitiesOf(preparing(t), owner(t))
	locked := room.CapabilitiesOf(publish(t, fullyDecided(t)), owner(t))
	decided := room.CapabilitiesOf(evolveChoicesInPreparing(t), owner(t))
	closed, _ := apply(t, preparing(t), &room.CloseCommand{ClosedBy: owner(t)})

	require.Equal(t, room.Capabilities{ChangeSeats: true, ChangeMatchRules: true, ChangeSeatChoice: []bool{true, true}, Publish: true, Close: true, Rename: true}, draft)
	require.Equal(t, room.Capabilities{ChangeMatchRules: true, ChangeSeatChoice: []bool{true, true}, Close: true, Rename: true}, prepared)
	require.Equal(t, room.Capabilities{ChangeSeatChoice: []bool{false, false}, DeclareReady: true, Close: true, Rename: true}, locked)
	require.Equal(t, room.Capabilities{ChangeMatchRules: true, ChangeSeatChoice: []bool{true, true}, DeclareReady: true, Close: true, Rename: true}, decided)
	require.Equal(t, room.Capabilities{ChangeSeatChoice: []bool{false, false}}, room.CapabilitiesOf(closed, owner(t)))
}

// 対戦が始まって終わった（次を決める）部屋。guests が 1 なら、作った人と guest の 2 人がプレイヤーの席に座る
func decidingNext(t *testing.T, guests int) room.State {
	t.Helper()
	var state room.State
	if guests == 0 {
		state, _ = apply(t, publish(t, fullyDecided(t)), &room.DeclareReadyCommand{Player: owner(t), Match: matchID(t)})
	} else {
		state = evolveChoice(t, preparingWithGuestSeat(t), guest(t), 0, playerChoice(t))
		state, _ = apply(t, state, &room.DeclareReadyCommand{Player: guest(t)})
		state = evolve(t, state, &room.ReadyDeclaredEvent{EventMeta: room.EventMeta{EntityID: roomtest.ID(t, 30), AggregateVersion: state.(room.Room).Version + 1, OccurredAt: changedAt}, Player: owner(t)})
		state = evolve(t, state, &room.MatchStartedEvent{EventMeta: room.EventMeta{EntityID: roomtest.ID(t, 30), AggregateVersion: state.(room.Room).Version + 1, OccurredAt: changedAt}, Match: matchID(t)})
	}
	state, _ = apply(t, state, &room.FinishMatchCommand{Match: matchID(t)})
	return state
}

func TestDecide_FinishMatchMovesPlayingRoomToDecidingNext(t *testing.T) {
	playing, _ := apply(t, publish(t, fullyDecided(t)), &room.DeclareReadyCommand{Player: owner(t), Match: matchID(t)})

	finished, events := apply(t, playing, &room.FinishMatchCommand{Match: matchID(t)})

	require.Equal(t, []room.Event{
		&room.MatchFinishedEvent{EventMeta: room.EventMeta{EntityID: roomtest.ID(t, 30), AggregateVersion: 10, OccurredAt: changedAt}, Match: matchID(t)},
	}, events)
	require.Equal(t, room.StatusDecidingNext, finished.(room.Room).Status)
}

func TestDecide_FinishMatchEmitsNothingUnlessItIsThePlayingRoomsCurrentMatch(t *testing.T) {
	playing, _ := apply(t, publish(t, fullyDecided(t)), &room.DeclareReadyCommand{Player: owner(t), Match: matchID(t)})
	tests := []struct {
		name  string
		state room.State
		match match.ID
	}{
		{name: "another_match", state: playing, match: matchtest.ID(t, 41)},
		{name: "second_time", state: decidingNext(t, 0), match: matchID(t)},
		{name: "preparing", state: publish(t, fullyDecided(t)), match: matchID(t)},
		{name: "draft", state: fullyDecided(t), match: matchID(t)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := room.Decide(tt.state, &room.FinishMatchCommand{Match: tt.match}, changedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

func TestDecide_DeclareNextDecidesAtOnceWhenOnlyOnePlayerIsInTheRoom(t *testing.T) {
	deciding := decidingNext(t, 0)

	paused, pauseEvents := apply(t, deciding, &room.DeclareNextCommand{Player: owner(t), Continue: false})
	restarted, restartEvents := apply(t, deciding, &room.DeclareNextCommand{Player: owner(t), Continue: true})

	version := deciding.(room.Room).Version
	meta := func(offset int64) room.EventMeta {
		return room.EventMeta{EntityID: roomtest.ID(t, 30), AggregateVersion: version + offset, OccurredAt: changedAt}
	}
	require.Equal(t, []room.Event{
		&room.NextDeclaredEvent{EventMeta: meta(1), Player: owner(t), Continue: false},
		&room.PausedEvent{EventMeta: meta(2)},
	}, pauseEvents)
	require.Equal(t, room.StatusPaused, paused.(room.Room).Status)
	require.Equal(t, []room.Event{
		&room.NextDeclaredEvent{EventMeta: meta(1), Player: owner(t), Continue: true},
		&room.PreparationRestartedEvent{EventMeta: meta(2)},
	}, restartEvents)
	require.Equal(t, room.StatusPreparing, restarted.(room.Room).Status)
}

func TestDecide_DeclareNextWaitsForEveryoneToContinueButPausesAsSoonAsSomeoneEnds(t *testing.T) {
	deciding := decidingNext(t, 1)

	waiting, events := apply(t, deciding, &room.DeclareNextCommand{Player: owner(t), Continue: true})
	restarted, _ := apply(t, waiting, &room.DeclareNextCommand{Player: guest(t), Continue: true})
	paused, _ := apply(t, deciding, &room.DeclareNextCommand{Player: guest(t), Continue: false})

	require.Len(t, events, 1)
	require.Equal(t, room.StatusDecidingNext, waiting.(room.Room).Status)
	require.Equal(t, []room.NextDeclaration{{Player: owner(t), Continue: true}}, waiting.(room.Room).NextDeclarations)
	require.Equal(t, room.StatusPreparing, restarted.(room.Room).Status)
	require.Equal(t, room.StatusPaused, paused.(room.Room).Status)
}

func TestDecide_DeclareNextRejectsOutsidersRepeatsAndOtherStatuses(t *testing.T) {
	waiting, _ := apply(t, decidingNext(t, 1), &room.DeclareNextCommand{Player: owner(t), Continue: true})
	playing, _ := apply(t, publish(t, fullyDecided(t)), &room.DeclareReadyCommand{Player: owner(t), Match: matchID(t)})
	tests := []struct {
		name   string
		state  room.State
		player player.ID
		want   error
	}{
		{name: "not_participant", state: decidingNext(t, 0), player: guest(t), want: room.ErrNotParticipant},
		{name: "already_declared", state: waiting, player: owner(t), want: room.ErrAlreadyDeclaredNext},
		{name: "playing", state: playing, player: owner(t), want: room.ErrStatusNotAllowed},
		{name: "preparing", state: publish(t, fullyDecided(t)), player: owner(t), want: room.ErrStatusNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := room.Decide(tt.state, &room.DeclareNextCommand{Player: tt.player, Continue: true}, changedAt)

			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestDecide_ContinuingReturnsToPreparingWithReadyAndLocksCleared(t *testing.T) {
	restarted, _ := apply(t, decidingNext(t, 0), &room.DeclareNextCommand{Player: owner(t), Continue: true})

	current := restarted.(room.Room)

	require.Empty(t, current.Ready)
	require.Empty(t, current.NextDeclarations)
	require.False(t, current.MatchRulesLocked)
	for _, seat := range current.Seats {
		require.False(t, seat.ChoiceLocked)
		require.NotNil(t, seat.Choice)
	}
}

func TestDecide_AfterContinuingConditionsCanBeChangedByTheirOwnersAndTheNextMatchCanStart(t *testing.T) {
	restarted, _ := apply(t, decidingNext(t, 0), &room.DeclareNextCommand{Player: owner(t), Continue: true})
	otherRules := roomtest.MatchRules(t, boardtest.ID(t, 6))

	state, rulesEvents := apply(t, restarted, &room.ChangeMatchRulesCommand{ChangedBy: owner(t), MatchRules: otherRules})
	state, choiceEvents := apply(t, state, &room.ChangeSeatChoiceCommand{ChangedBy: owner(t), Seat: roomtest.SeatIndex(t, 0), Choice: roomtest.PlayerChoice(t, 8, 9)})
	_, seatsErr := room.Decide(state, &room.ChangeSeatsCommand{ChangedBy: owner(t), Seats: seatKinds(t)}, changedAt)
	state, startEvents := apply(t, state, &room.DeclareReadyCommand{Player: owner(t), Match: matchtest.ID(t, 41)})

	require.Len(t, rulesEvents, 1)
	require.Len(t, choiceEvents, 1)
	require.ErrorIs(t, seatsErr, room.ErrStatusNotAllowed)
	require.Len(t, startEvents, 2)
	require.Equal(t, room.StatusPlaying, state.(room.Room).Status)
	require.Equal(t, matchtest.ID(t, 41), state.(room.Room).Match)
}

func TestCapabilitiesOf_ShowsDeclareNextOnlyToThoseWhoHaveNotDeclaredYet(t *testing.T) {
	waiting, _ := apply(t, decidingNext(t, 1), &room.DeclareNextCommand{Player: owner(t), Continue: true})

	require.Equal(t, room.Capabilities{ChangeSeatChoice: []bool{false, false}, DeclareNext: true, Close: true, Rename: true}, room.CapabilitiesOf(decidingNext(t, 1), owner(t)))
	require.False(t, room.CapabilitiesOf(waiting, owner(t)).DeclareNext)
	require.True(t, room.CapabilitiesOf(waiting, guest(t)).DeclareNext)
}

// 終えた部屋（休止中）。guests が 1 なら、作った人と guest の 2 人がプレイヤーの席に座る
func paused(t *testing.T, guests int) room.State {
	t.Helper()
	state, _ := apply(t, decidingNext(t, guests), &room.DeclareNextCommand{Player: owner(t), Continue: false})
	return state
}

func TestDecide_ResumeReturnsPausedRoomToPreparingAsContinuingDoes(t *testing.T) {
	before := paused(t, 0)

	resumed, events := apply(t, before, &room.ResumeCommand{ResumedBy: owner(t)})
	continued, _ := apply(t, decidingNext(t, 0), &room.DeclareNextCommand{Player: owner(t), Continue: true})

	require.Equal(t, []room.Event{
		&room.PreparationRestartedEvent{EventMeta: room.EventMeta{EntityID: roomtest.ID(t, 30), AggregateVersion: before.(room.Room).Version + 1, OccurredAt: changedAt}},
	}, events)
	current := resumed.(room.Room)
	require.Equal(t, room.StatusPreparing, current.Status)
	require.Empty(t, current.Ready)
	require.Empty(t, current.NextDeclarations)
	require.False(t, current.MatchRulesLocked)
	require.Equal(t, continued.(room.Room).Seats, current.Seats)
}

func TestDecide_ResumeRejectsAnyoneButTheOwnerAndRoomsThatAreNotPaused(t *testing.T) {
	tests := []struct {
		name   string
		state  room.State
		player player.ID
		want   error
	}{
		{name: "guest", state: paused(t, 1), player: guest(t), want: room.ErrNotOwner},
		{name: "deciding_next", state: decidingNext(t, 0), player: owner(t), want: room.ErrStatusNotAllowed},
		{name: "preparing", state: publish(t, fullyDecided(t)), player: owner(t), want: room.ErrStatusNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := room.Decide(tt.state, &room.ResumeCommand{ResumedBy: tt.player}, changedAt)

			require.ErrorIs(t, err, tt.want)
		})
	}
}

// 休止中は一覧に残る部屋で、閉じるのは作った人の閉じる操作だけ
func TestDecide_PausedRoomIsClosedOnlyByTheOwnersClose(t *testing.T) {
	closed, events := apply(t, paused(t, 0), &room.CloseCommand{ClosedBy: owner(t)})

	require.Len(t, events, 1)
	require.IsType(t, &room.ClosedEvent{}, events[0])
	require.Equal(t, room.StatusClosed, closed.(room.Room).Status)
	require.Equal(t, room.Capabilities{ChangeSeatChoice: []bool{false, false}, Resume: true, Close: true, Rename: true}, room.CapabilitiesOf(paused(t, 0), owner(t)))
}

func TestDecide_CreateRequiresAnAvatarAndADeckAndListsWhatIsMissing(t *testing.T) {
	tests := []struct {
		name    string
		avatars int
		decks   int
		want    []room.Requirement
	}{
		{name: "neither", avatars: 0, decks: 0, want: []room.Requirement{room.RequirementAvatar, room.RequirementDeck}},
		{name: "no_deck", avatars: 1, decks: 0, want: []room.Requirement{room.RequirementDeck}},
		{name: "both_owned", avatars: 2, decks: 3, want: nil},
		{name: "no_avatar", avatars: 0, decks: 1, want: []room.Requirement{room.RequirementAvatar}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := room.Decide(room.InitialState(), &room.CreateCommand{ID: roomtest.ID(t, 30), Owner: owner(t), Name: roomtest.Name(t, "部屋"), Avatars: tt.avatars, Decks: tt.decks}, changedAt)

			if tt.want == nil {
				require.NoError(t, err)
				require.Len(t, events, 1)
				require.Empty(t, room.MissingRequirements(tt.avatars, tt.decks))
				return
			}
			var notMet *room.RequirementsNotMetError
			require.ErrorAs(t, err, &notMet)
			require.ErrorIs(t, err, room.ErrRequirementsNotMet)
			require.Equal(t, tt.want, notMet.Missing)
			require.Equal(t, tt.want, room.MissingRequirements(tt.avatars, tt.decks))
			require.Empty(t, events)
		})
	}
}
