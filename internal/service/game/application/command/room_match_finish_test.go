package command_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
)

// 対戦中の部屋と、そのポリシー。部屋の read model の代わりに、渡した対戦の公開 ID を渡した部屋として返す
type roomMatchFinishFixture struct {
	roomFixture
	room         command.ChangedRoom
	matchID      match.ID
	matchPublic  uuid.UUID
	handler      *command.RoomMatchFinishHandler
	findRoomArgs []uuid.UUID
}

func newRoomMatchFinishFixture(t *testing.T, found bool) *roomMatchFinishFixture {
	t.Helper()
	f := &roomMatchFinishFixture{roomFixture: newRoomFixture(t)}
	current := f.fullyDecided(t)
	published, err := f.roomFixture.handler.Publish(f.owner, current.ID, current.Revision)
	require.NoError(t, err)
	f.room, err = f.roomFixture.handler.DeclareReady(f.owner, current.ID, published.Revision)
	require.NoError(t, err)
	f.matchID = f.load(t, current.ID).Match
	refs := identifier.NewEntityRefResolver(f.registry)
	ref := domain.NewEntityRef(f.matchID.EntityType(), f.matchID.Int64())
	publicIDs, err := refs.PublicIDs(f.ctx, []domain.EntityRef{ref})
	require.NoError(t, err)
	f.matchPublic = publicIDs[ref]
	f.handler = command.NewRoomMatchFinishHandler(
		eventstore.New(f.registry, room.NewID, room.NewEventType, room.InitialState, room.Evolve),
		identifier.New(f.registry, room.NewID),
		refs,
		func(_ context.Context, matchID uuid.UUID) (uuid.UUID, int, bool, error) {
			f.findRoomArgs = append(f.findRoomArgs, matchID)
			return f.room.ID, 1, found, nil
		},
		func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	)
	return f
}

func (f *roomMatchFinishFixture) rewardsCompleted() *match.AllRewardsReceivedEvent {
	return &match.AllRewardsReceivedEvent{EventMeta: match.EventMeta{EntityID: f.matchID, AggregateVersion: 30}}
}

func TestRoomMatchFinishHandler_MovesRoomOfTheMatchToDecidingNextOnRewardsCompleted(t *testing.T) {
	f := newRoomMatchFinishFixture(t, true)

	err := f.handler.Handle(f.ctx, f.rewardsCompleted())

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{f.matchPublic}, f.findRoomArgs)
	loaded := f.load(t, f.room.ID)
	require.Equal(t, room.StatusDecidingNext, loaded.Status)
	require.Equal(t, f.room.Revision+1, loaded.Version)
}

func TestRoomMatchFinishHandler_ChangesTheRoomOnlyOnceWhenItRunsTwice(t *testing.T) {
	f := newRoomMatchFinishFixture(t, true)

	require.NoError(t, f.handler.Handle(f.ctx, f.rewardsCompleted()))
	require.NoError(t, f.handler.Handle(f.ctx, f.rewardsCompleted()))

	require.Equal(t, f.room.Revision+1, f.load(t, f.room.ID).Version)
}

func TestRoomMatchFinishHandler_ReturnsRetryableFailureWhileTheRoomIsNotProjectedYet(t *testing.T) {
	f := newRoomMatchFinishFixture(t, false)

	err := f.handler.Handle(f.ctx, f.rewardsCompleted())

	require.ErrorIs(t, err, command.ErrRoomOfMatchNotFound)
	require.Equal(t, room.StatusPlaying, f.load(t, f.room.ID).Status)
}

func TestRoomMatchFinishHandler_IgnoresOtherEventsOfTheMatch(t *testing.T) {
	f := newRoomMatchFinishFixture(t, true)

	err := f.handler.Handle(f.ctx, &match.TurnEndedEvent{EventMeta: match.EventMeta{EntityID: f.matchID}})

	require.NoError(t, err)
	require.Empty(t, f.findRoomArgs)
	require.Equal(t, room.StatusPlaying, f.load(t, f.room.ID).Status)
}

func TestRoomMatchFinishHandler_PassesOnFailureToFindTheRoom(t *testing.T) {
	f := newRoomMatchFinishFixture(t, true)
	boom := errors.New("boom")
	handler := command.NewRoomMatchFinishHandler(
		eventstore.New(f.registry, room.NewID, room.NewEventType, room.InitialState, room.Evolve),
		identifier.New(f.registry, room.NewID),
		identifier.NewEntityRefResolver(f.registry),
		func(context.Context, uuid.UUID) (uuid.UUID, int, bool, error) { return uuid.Nil, 0, false, boom },
		time.Now,
	)

	require.ErrorIs(t, handler.Handle(f.ctx, f.rewardsCompleted()), boom)
}

// 対戦の後の意思表示は、部屋の操作として作った人が行う。終えるなら休止中に、続けるなら準備中に戻る
func (f *roomMatchFinishFixture) decidingNext(t *testing.T) command.ChangedRoom {
	t.Helper()
	require.NoError(t, f.handler.Handle(f.ctx, f.rewardsCompleted()))
	return command.ChangedRoom{ID: f.room.ID, Revision: f.load(t, f.room.ID).Version}
}

func TestRoomDeclareNext_ContinuingReturnsToPreparingAndClearsReady(t *testing.T) {
	f := newRoomMatchFinishFixture(t, true)
	deciding := f.decidingNext(t)

	declared, err := f.roomFixture.handler.DeclareNext(f.owner, deciding.ID, deciding.Revision, true)

	require.NoError(t, err)
	loaded := f.load(t, deciding.ID)
	require.Equal(t, room.StatusPreparing, loaded.Status)
	require.Empty(t, loaded.Ready)
	// 意思表示と準備中に戻るの 2 つのイベント分進む
	require.Equal(t, deciding.Revision+2, declared.Revision)
}

func TestRoomDeclareNext_EndingPausesTheRoomAndTheOwnerCanResumeIt(t *testing.T) {
	f := newRoomMatchFinishFixture(t, true)
	other := registeredPlayer(t, f.ctx, "other")
	deciding := f.decidingNext(t)

	ended, err := f.roomFixture.handler.DeclareNext(f.owner, deciding.ID, deciding.Revision, false)
	require.NoError(t, err)
	pausedStatus := f.load(t, deciding.ID).Status
	_, stale := f.roomFixture.handler.Resume(f.owner, deciding.ID, deciding.Revision)
	_, outsider := f.roomFixture.handler.Resume(other, deciding.ID, ended.Revision)
	resumed, resumeErr := f.roomFixture.handler.Resume(f.owner, deciding.ID, ended.Revision)

	require.Equal(t, room.StatusPaused, pausedStatus)
	require.ErrorIs(t, stale, command.ErrRevisionConflict)
	require.ErrorIs(t, outsider, command.ErrRoomNotFound)
	require.NoError(t, resumeErr)
	require.Equal(t, ended.Revision+1, resumed.Revision)
	require.Equal(t, room.StatusPreparing, f.load(t, deciding.ID).Status)
}

func TestRoomResume_RejectsRoomsThatAreNotPaused(t *testing.T) {
	f := newRoomMatchFinishFixture(t, true)
	deciding := f.decidingNext(t)

	_, err := f.roomFixture.handler.Resume(f.owner, deciding.ID, deciding.Revision)

	require.ErrorIs(t, err, command.ErrRoomStatusNotAllowed)
}

func TestRoomDeclareNext_RejectsOutsideDecidingNextAndStaleRevisionAndOutsiders(t *testing.T) {
	f := newRoomMatchFinishFixture(t, true)
	other := registeredPlayer(t, f.ctx, "other")

	_, whilePlaying := f.roomFixture.handler.DeclareNext(f.owner, f.room.ID, f.room.Revision, true)
	deciding := f.decidingNext(t)
	_, stale := f.roomFixture.handler.DeclareNext(f.owner, deciding.ID, deciding.Revision-1, true)
	_, outsider := f.roomFixture.handler.DeclareNext(other, deciding.ID, deciding.Revision, true)

	require.ErrorIs(t, whilePlaying, command.ErrRoomStatusNotAllowed)
	require.ErrorIs(t, stale, command.ErrRevisionConflict)
	require.ErrorIs(t, outsider, command.ErrRoomNotFound)
}
