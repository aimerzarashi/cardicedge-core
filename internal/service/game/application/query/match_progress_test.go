package query_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match/matchtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

const receiveTimeout = 5 * time.Second

var progressBase = time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)

// DB の代わりに、今の最新のイベントを覚えておく。読み直した回数も数える
type fakeMatchHead struct {
	mu    sync.Mutex
	head  query.MatchHead
	err   error
	loads int
}

func (f *fakeMatchHead) set(revision int64, ended bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.head = query.MatchHead{Revision: revision, OccurredAt: progressBase.Add(time.Duration(revision) * time.Second), Ended: ended}
}

func (f *fakeMatchHead) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeMatchHead) load(context.Context, match.ID) (query.MatchHead, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loads++
	return f.head, f.err
}

func (f *fakeMatchHead) loadCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loads
}

type progressFixture struct {
	t        *testing.T
	head     *fakeMatchHead
	broker   *query.MatchProgressBroker
	matchID  uuid.UUID
	internal match.ID
}

// interval は送り直す間隔。長くすると、通知だけで届くことを確かめられる
func newProgressFixture(t *testing.T, interval time.Duration) *progressFixture {
	t.Helper()
	f := &progressFixture{t: t, head: &fakeMatchHead{}, matchID: uuid.New(), internal: matchtest.ID(t, 7)}
	f.head.set(3, false)
	authorize := func(_ context.Context, _ uuid.UUID, matchID uuid.UUID) (match.ID, error) {
		if matchID != f.matchID {
			return 0, query.ErrMatchNotFound
		}
		return f.internal, nil
	}
	f.broker = query.NewMatchProgressBroker(authorize, f.head.load, interval)
	t.Cleanup(f.broker.Close)
	return f
}

func (f *progressFixture) subscribe(ctx context.Context) <-chan query.MatchProgress {
	f.t.Helper()
	updates, err := f.broker.Subscribe(ctx, uuid.New(), f.matchID)
	require.NoError(f.t, err)
	return updates
}

func receive(t *testing.T, updates <-chan query.MatchProgress) query.MatchProgress {
	t.Helper()
	select {
	case progress, ok := <-updates:
		require.True(t, ok)
		return progress
	case <-time.After(receiveTimeout):
		require.FailNow(t, "timed out waiting for progress")
		return query.MatchProgress{}
	}
}

// 送り直しの分（同じ版）を読み飛ばして、その版が届くまで読む
func receiveUntilRevision(t *testing.T, updates <-chan query.MatchProgress, revision int64) {
	t.Helper()
	for range 1000 {
		if receive(t, updates).Revision == revision {
			return
		}
	}
	require.FailNow(t, "revision did not arrive")
}

func requireClosed(t *testing.T, updates <-chan query.MatchProgress) {
	t.Helper()
	select {
	case _, ok := <-updates:
		require.False(t, ok)
	case <-time.After(receiveTimeout):
		require.FailNow(t, "timed out waiting for the channel to close")
	}
}

func requireNothing(t *testing.T, updates <-chan query.MatchProgress, within time.Duration) {
	t.Helper()
	select {
	case <-updates:
		require.FailNow(t, "unexpected activity")
	case <-time.After(within):
	}
}

func TestMatchProgressBroker_SendsTheCurrentRevisionOnSubscribe(t *testing.T) {
	f := newProgressFixture(t, time.Hour)

	progress := receive(t, f.subscribe(t.Context()))

	require.Equal(t, query.MatchProgress{ID: f.matchID, Revision: 3, OccurredAt: progressBase.Add(3 * time.Second)}, progress)
}

func TestMatchProgressBroker_SendsTheAdvancedRevisionOnNotify(t *testing.T) {
	f := newProgressFixture(t, time.Hour)
	updates := f.subscribe(t.Context())
	receive(t, updates)

	f.head.set(4, false)
	f.broker.Notify(f.internal, 4)

	require.Equal(t, int64(4), receive(t, updates).Revision)
}

// 受け手が受け取れない間に進んだ版は、まとめて最新の 1 つだけを送る（古い版を送り直さない）
func TestMatchProgressBroker_CoalescesRevisionsWhileTheReceiverIsBusy(t *testing.T) {
	f := newProgressFixture(t, time.Hour)
	updates := f.subscribe(t.Context())
	first := receive(t, updates)

	for revision := int64(4); revision <= 9; revision++ {
		f.head.set(revision, false)
		f.broker.Notify(f.internal, revision)
	}

	require.Equal(t, int64(3), first.Revision)
	require.Equal(t, int64(9), receive(t, updates).Revision)
	requireNothing(t, updates, 50*time.Millisecond)
}

// 通知が届かなくても、間隔ごとに DB から読み直して送る（取りこぼしを埋め、変わらなくても送る）
func TestMatchProgressBroker_RereadsAndResendsOnTheIntervalWithoutNotifications(t *testing.T) {
	f := newProgressFixture(t, 20*time.Millisecond)
	updates := f.subscribe(t.Context())
	require.Equal(t, int64(3), receive(t, updates).Revision)

	require.Equal(t, int64(3), receive(t, updates).Revision)
	f.head.set(5, false)

	receiveUntilRevision(t, updates, 5)
}

func TestMatchProgressBroker_IgnoresNotificationsForRevisionsAlreadySent(t *testing.T) {
	f := newProgressFixture(t, time.Hour)
	updates := f.subscribe(t.Context())
	receive(t, updates)
	loads := f.head.loadCount()

	f.broker.Notify(f.internal, 3)
	f.broker.Notify(f.internal, 2)

	requireNothing(t, updates, 50*time.Millisecond)
	require.Equal(t, loads, f.head.loadCount())
}

func TestMatchProgressBroker_OnlyNotifiesSubscribersOfThatMatch(t *testing.T) {
	f := newProgressFixture(t, time.Hour)
	updates := f.subscribe(t.Context())
	receive(t, updates)

	f.head.set(4, false)
	f.broker.Notify(matchtest.ID(t, 8), 4)

	requireNothing(t, updates, 50*time.Millisecond)
}

func TestMatchProgressBroker_ClosesAfterSendingTheEndedRevision(t *testing.T) {
	f := newProgressFixture(t, time.Hour)
	updates := f.subscribe(t.Context())
	receive(t, updates)

	f.head.set(10, true)
	f.broker.Notify(f.internal, 10)

	require.Equal(t, int64(10), receive(t, updates).Revision)
	requireClosed(t, updates)
}

func TestMatchProgressBroker_SendsTheEndedRevisionAndClosesForAMatchThatHasAlreadyEnded(t *testing.T) {
	f := newProgressFixture(t, time.Hour)
	f.head.set(10, true)

	updates := f.subscribe(t.Context())

	require.Equal(t, int64(10), receive(t, updates).Revision)
	requireClosed(t, updates)
}

func TestMatchProgressBroker_RejectsMatchesTheViewerCannotRead(t *testing.T) {
	f := newProgressFixture(t, time.Hour)

	_, err := f.broker.Subscribe(t.Context(), uuid.New(), uuid.New())

	require.ErrorIs(t, err, query.ErrMatchNotFound)
}

func TestMatchProgressBroker_ReturnsTheErrorWhenTheFirstReadFails(t *testing.T) {
	f := newProgressFixture(t, time.Hour)
	failure := errors.New("db down")
	f.head.setErr(failure)

	_, err := f.broker.Subscribe(t.Context(), uuid.New(), f.matchID)

	require.ErrorIs(t, err, failure)
}

func TestMatchProgressBroker_KeepsTheSubscriptionWhenARereadFails(t *testing.T) {
	f := newProgressFixture(t, 20*time.Millisecond)
	updates := f.subscribe(t.Context())
	receive(t, updates)

	f.head.setErr(errors.New("db down"))
	time.Sleep(60 * time.Millisecond)
	f.head.setErr(nil)
	f.head.set(6, false)

	receiveUntilRevision(t, updates, 6)
}

// 閉じたら購読者が外れ、通知で読み直されなくなる
func TestMatchProgressBroker_RemovesTheSubscriberWhenItsContextEnds(t *testing.T) {
	f := newProgressFixture(t, time.Hour)
	ctx, cancel := context.WithCancel(t.Context())
	updates := f.subscribe(ctx)
	receive(t, updates)

	cancel()
	requireClosed(t, updates)
	loads := f.head.loadCount()
	f.head.set(4, false)
	f.broker.Notify(f.internal, 4)

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, loads, f.head.loadCount())
}

func TestMatchProgressBroker_CloseEndsEverySubscriptionAndRefusesNewOnes(t *testing.T) {
	f := newProgressFixture(t, time.Hour)
	first := f.subscribe(t.Context())
	second := f.subscribe(t.Context())
	receive(t, first)
	receive(t, second)

	f.broker.Close()

	requireClosed(t, first)
	requireClosed(t, second)
	late := f.subscribe(t.Context())
	requireClosed(t, late)
}
