package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match/matchtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
)

// 対戦中になった部屋（最後の準備完了まで進めたもの）と、その部屋から対戦を始めるハンドラー
type matchStartFixture struct {
	roomFixture
	room    room.Room
	handler *command.MatchStartHandler
	matches func(t *testing.T) match.State
}

func newMatchStartFixture(t *testing.T) matchStartFixture {
	t.Helper()
	f := matchStartFixture{roomFixture: newRoomFixture(t)}
	current := f.fullyDecided(t)
	published, err := f.roomFixture.handler.Publish(f.owner, current.ID, current.Revision)
	require.NoError(t, err)
	ready, err := f.roomFixture.handler.DeclareReady(f.owner, current.ID, published.Revision)
	require.NoError(t, err)
	f.room = f.load(t, ready.ID)

	matchStore := eventstore.New(f.registry, match.NewID, match.NewEventType, match.InitialState, match.Evolve)
	base := boardtest.Tile(t, 0, 0, "base", "")
	snapshot := match.Board{
		ID: f.room.MatchRules.Board, Size: boardtest.Size(t, 2, 1),
		Tiles:       []board.Tile{base, boardtest.Tile(t, 1, 0, "land", "fire")},
		Connections: []board.Connection{boardtest.Connection(t, 0, 0, 1, 0)},
	}
	f.handler = command.NewMatchStartHandler(
		eventstore.New(f.registry, room.NewID, room.NewEventType, room.InitialState, room.Evolve),
		matchStore,
		func(context.Context) (gamerules.Rules, bool, error) { return gamerules.Defaults(), true, nil },
		func(context.Context, board.ID) (match.Board, error) { return snapshot, nil },
		func(context.Context, deck.ID) ([]master.DeckCard, error) {
			return []master.DeckCard{{Card: cardA, Count: 4}, {Card: cardB, Count: 3}}, nil
		},
		func(context.Context, npcdeck.ID) ([]master.DeckCard, error) {
			return []master.DeckCard{{Card: cardC, Count: 2}}, nil
		},
		func() match.Random {
			rules := matchtest.Rules(t, 1, 6, 3)
			return matchtest.NewRandom(t, rules, 2, 5)
		},
		func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	)
	f.matches = func(t *testing.T) match.State {
		t.Helper()
		state, err := matchStore.Load(f.ctx, f.room.Match)
		require.NoError(t, err)
		return state
	}
	return f
}

func TestMatchStart_CreatesMatchWithIDOfRoomEventFromRoomConditions(t *testing.T) {
	f := newMatchStartFixture(t)

	err := f.handler.Handle(f.ctx, &room.MatchStartedEvent{EventMeta: room.EventMeta{EntityID: f.room.ID}, Match: f.room.Match})

	require.NoError(t, err)
	started, ok := f.matches(t).(match.Match)
	require.True(t, ok)
	require.Equal(t, f.room.Match, started.ID)
	require.Len(t, started.Participants, 2)
	require.NotNil(t, started.Participants[0].Participant.Player)
	require.Equal(t, f.room.Owner, started.Participants[0].Participant.Player.Player)
	require.NotNil(t, started.Participants[1].Participant.NPC)
	require.Equal(t, f.room.MatchRules.Board, started.Board.ID)
	require.Equal(t, f.room.MatchRules.TargetMagic, started.Rules.TargetMagic)
	// 山札の枚数は、デッキの枚数から配った手札を引いた数
	require.Len(t, started.Participants[0].Hand, 5)
	require.Len(t, started.Participants[0].DrawPile, 2)
	require.Len(t, started.Participants[1].Hand, 2)
	require.Empty(t, started.Participants[1].DrawPile)
}

func TestMatchStart_IsIdempotent(t *testing.T) {
	f := newMatchStartFixture(t)
	event := &room.MatchStartedEvent{EventMeta: room.EventMeta{EntityID: f.room.ID}, Match: f.room.Match}
	require.NoError(t, f.handler.Handle(f.ctx, event))
	before, ok := f.matches(t).(match.Match)
	require.True(t, ok)

	err := f.handler.Handle(f.ctx, event)

	require.NoError(t, err)
	after, ok := f.matches(t).(match.Match)
	require.True(t, ok)
	require.Equal(t, before.Version, after.Version)
}

func TestMatchStart_IgnoresOtherRoomEvents(t *testing.T) {
	f := newMatchStartFixture(t)

	err := f.handler.Handle(f.ctx, &room.PublishedEvent{EventMeta: room.EventMeta{EntityID: f.room.ID}})

	require.NoError(t, err)
	_, uncreated := f.matches(t).(match.UncreatedMatch)
	require.True(t, uncreated)
}
