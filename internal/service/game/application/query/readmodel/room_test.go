package readmodel_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room/roomtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

var (
	roomCreatedAt = time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	roomChangedAt = roomCreatedAt.Add(time.Hour)
)

func roomMeta(t *testing.T, version int64, at time.Time) room.EventMeta {
	t.Helper()
	return room.EventMeta{EntityID: roomtest.ID(t, 3), AggregateVersion: version, OccurredAt: at}
}

func TestRoom_CreatedMakesDraftRowWithoutSeatsOwnedByPlayer(t *testing.T) {
	owner := playertest.ID(t, 7)

	state := readmodel.Room{}.Evolve(&room.CreatedEvent{EventMeta: roomMeta(t, 1, roomCreatedAt), Player: owner, Name: roomtest.Name(t, "部屋")})

	require.Equal(t, readmodel.Room{
		Room: roomtest.ID(t, 3), Owner: owner, Name: roomtest.Name(t, "部屋"), Status: readmodel.RoomStatusDraft, Revision: 1,
		Seats: []readmodel.RoomSeat{}, Ready: []player.ID{},
		RegisteredAt: roomCreatedAt, UpdatedAt: roomCreatedAt, UpdatedBy: owner,
	}, state)
	require.False(t, state.IsTerminal())
}

func TestRoom_EvolvesThroughEveryEventToPreparingAndReady(t *testing.T) {
	owner := playertest.ID(t, 7)
	matchRules := roomtest.MatchRules(t, boardtest.ID(t, 5))
	seats := []room.Seat{{Kind: room.SeatKindPlayer, Player: owner}, {Kind: room.SeatKindNPC}}

	state := readmodel.Room{}.
		Evolve(&room.CreatedEvent{EventMeta: roomMeta(t, 1, roomCreatedAt), Player: owner, Name: roomtest.Name(t, "部屋")}).
		Evolve(&room.SeatsChangedEvent{EventMeta: roomMeta(t, 2, roomChangedAt), Seats: seats}).
		Evolve(&room.MatchRulesChangedEvent{EventMeta: roomMeta(t, 3, roomChangedAt), MatchRules: matchRules}).
		Evolve(&room.SeatChoiceChangedEvent{EventMeta: roomMeta(t, 4, roomChangedAt), Seat: roomtest.SeatIndex(t, 0), Choice: roomtest.PlayerChoice(t, 1, 2), ChangedBy: owner}).
		Evolve(&room.SeatChoiceChangedEvent{EventMeta: roomMeta(t, 5, roomChangedAt), Seat: roomtest.SeatIndex(t, 1), Choice: roomtest.NPCChoice(t, 3, 4), ChangedBy: owner})
	// 公開しただけでは状態は変わらない（定員になって準備中に進む）
	published := state.Evolve(&room.PublishedEvent{EventMeta: roomMeta(t, 6, roomChangedAt)})
	preparing := published.Evolve(&room.PreparationStartedEvent{EventMeta: roomMeta(t, 7, roomChangedAt)})
	ready := preparing.Evolve(&room.ReadyDeclaredEvent{EventMeta: roomMeta(t, 8, roomChangedAt), Player: owner})
	renamed := ready.Evolve(&room.RenamedEvent{EventMeta: roomMeta(t, 9, roomChangedAt), Name: roomtest.Name(t, "新しい名前")})

	require.Equal(t, readmodel.RoomStatusDraft, published.Status)
	require.Equal(t, readmodel.RoomStatusPreparing, preparing.Status)
	require.Equal(t, []player.ID{owner}, ready.Ready)
	require.Equal(t, int64(8), ready.Revision)
	require.Equal(t, roomCreatedAt, ready.RegisteredAt)
	require.Equal(t, roomChangedAt, ready.UpdatedAt)
	require.Equal(t, matchRules, *ready.MatchRules)
	require.Equal(t, roomtest.PlayerChoice(t, 1, 2), *ready.Seats[0].Choice)
	require.Equal(t, roomtest.NPCChoice(t, 3, 4), *ready.Seats[1].Choice)
	require.Equal(t, owner, ready.Seats[0].Player)
	require.Equal(t, "部屋", ready.Name.String())
	require.Equal(t, "新しい名前", renamed.Name.String())
	require.Equal(t, int64(9), renamed.Revision)
	require.Equal(t, readmodel.RoomMetadata{
		Name: "部屋", Status: readmodel.RoomStatusPreparing, Revision: 8, RegisteredAt: roomCreatedAt, UpdatedAt: roomChangedAt,
		MatchRules: &readmodel.RoomMatchRulesMetadata{
			TargetMagic: matchRules.TargetMagic, StartingMagic: matchRules.StartingMagic, LapBonus: matchRules.LapBonus,
			CheckpointBonus: matchRules.CheckpointBonus, RoundLimit: matchRules.RoundLimit, DiceMin: matchRules.DiceMin, DiceMax: matchRules.DiceMax,
		},
	}, ready.Metadata())
}

func TestRoom_ChangingSeatsClearsChoicesAndClosedEventClosesRowWithoutDeleting(t *testing.T) {
	owner := playertest.ID(t, 7)
	seats := []room.Seat{{Kind: room.SeatKindPlayer, Player: owner}, {Kind: room.SeatKindNPC}}
	chosen := readmodel.Room{}.
		Evolve(&room.CreatedEvent{EventMeta: roomMeta(t, 1, roomCreatedAt), Player: owner}).
		Evolve(&room.SeatsChangedEvent{EventMeta: roomMeta(t, 2, roomChangedAt), Seats: seats}).
		Evolve(&room.SeatChoiceChangedEvent{EventMeta: roomMeta(t, 3, roomChangedAt), Seat: roomtest.SeatIndex(t, 0), Choice: roomtest.PlayerChoice(t, 1, 2), ChangedBy: owner})

	changed := chosen.Evolve(&room.SeatsChangedEvent{EventMeta: roomMeta(t, 4, roomChangedAt), Seats: seats})
	closed := changed.Evolve(&room.ClosedEvent{EventMeta: roomMeta(t, 5, roomChangedAt)})

	require.NotNil(t, chosen.Seats[0].Choice)
	require.Nil(t, changed.Seats[0].Choice)
	require.Equal(t, readmodel.RoomStatusClosed, closed.Status)
	require.False(t, closed.IsTerminal())
}

func TestRoom_DoesNotChangeGivenStateAndIgnoresOtherAggregateEvents(t *testing.T) {
	owner := playertest.ID(t, 7)
	seats := []room.Seat{{Kind: room.SeatKindPlayer, Player: owner}, {Kind: room.SeatKindNPC}}
	state := readmodel.Room{}.
		Evolve(&room.CreatedEvent{EventMeta: roomMeta(t, 1, roomCreatedAt), Player: owner}).
		Evolve(&room.SeatsChangedEvent{EventMeta: roomMeta(t, 2, roomChangedAt), Seats: seats})

	state.Evolve(&room.SeatChoiceChangedEvent{EventMeta: roomMeta(t, 3, roomChangedAt), Seat: roomtest.SeatIndex(t, 0), Choice: roomtest.PlayerChoice(t, 1, 2), ChangedBy: owner})
	ignored := state.Evolve(&player.RegisteredEvent{EventMeta: player.EventMeta{EntityID: owner, AggregateVersion: 1}})

	require.Nil(t, state.Seats[0].Choice)
	require.Equal(t, state, ignored)
}

func TestNewRoomStatus_RejectsUnknownValue(t *testing.T) {
	_, err := readmodel.NewRoomStatus("recruiting")

	require.Error(t, err)
}
