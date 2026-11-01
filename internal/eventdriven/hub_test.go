package eventdriven_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/logtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

// 書かれた read model を受け取る。panicAt 件目のイベントまで当てた状態を書くときは panic する（0 なら panic しない）。
// panicOnce なら最初の 1 回だけ panic する（試し直しで直る失敗）
type recorder struct {
	upserted  chan countingReadModel
	panicAt   int
	panicOnce bool
	panicked  atomic.Bool
}

func (r *recorder) spec() eventdriven.ProjectionSpec[countingReadModel] {
	return eventdriven.ProjectionSpec[countingReadModel]{
		Name:          testProjectionName,
		Subscriptions: []domain.EntityType{operator.Types().EntityType},
		Upsert: func(_ context.Context, state countingReadModel) error {
			if r.panicAt > 0 && state.Applied == r.panicAt && !(r.panicOnce && r.panicked.Swap(true)) {
				panic("eventdriven test panic")
			}
			r.upserted <- state
			return nil
		},
		Delete: func(context.Context, countingReadModel) error {
			return nil
		},
	}
}

func (r *recorder) next(t *testing.T) countingReadModel {
	t.Helper()
	select {
	case state := <-r.upserted:
		return state
	case <-time.After(receiveTimeout):
		t.Fatal("no read model upserted")
		return countingReadModel{}
	}
}

// 本物の EventLoader を包み、集約ごとに読み込みの回数を数える。hold の集約を読むときは、release されるまで止まる。
// ワーカーを止めておけるため、知らせとワーカーの動く順番をテストが決められる（待ち時間に頼らない）
type probeLoader struct {
	eventstore.EventLoader
	mu       sync.Mutex
	counts   map[int64]int
	loaded   chan int64
	hold     atomic.Int64
	entered  chan struct{}
	released chan struct{}
}

func newProbeLoader(loader eventstore.EventLoader) *probeLoader {
	return &probeLoader{
		EventLoader: loader,
		counts:      make(map[int64]int),
		loaded:      make(chan int64, 100),
		entered:     make(chan struct{}),
		released:    make(chan struct{}),
	}
}

func (l *probeLoader) LoadEvents(ctx context.Context, entityID int64) ([]eventstore.LoadedEvent, error) {
	l.mu.Lock()
	l.counts[entityID]++
	l.mu.Unlock()
	if l.hold.CompareAndSwap(entityID, 0) {
		l.entered <- struct{}{}
		<-l.released
	}
	l.loaded <- entityID
	return l.EventLoader.LoadEvents(ctx, entityID)
}

func (l *probeLoader) count(entityID int64) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.counts[entityID]
}

// entityID の読み込みが始まるまで待つ（ほかの集約の読み込みは読み飛ばす）
func (l *probeLoader) waitLoaded(t *testing.T, entityID int64) {
	t.Helper()
	for {
		select {
		case loaded := <-l.loaded:
			if loaded == entityID {
				return
			}
		case <-time.After(receiveTimeout):
			t.Fatalf("entity %d was not loaded", entityID)
		}
	}
}

// 次に entityID を読むワーカーを止め、止まるまで待つ。返した関数で動かす
func (l *probeLoader) holdNext(t *testing.T, entityID int64, trigger func()) func() {
	t.Helper()
	l.hold.Store(entityID)
	trigger()
	select {
	case <-l.entered:
	case <-time.After(receiveTimeout):
		t.Fatalf("entity %d was not held", entityID)
	}
	return func() {
		l.released <- struct{}{}
	}
}

type hubFixture struct {
	ctx      context.Context
	db       *sql.DB
	dsn      string
	registry *domaindef.Registry
	hub      *eventdriven.Hub
	recorder *recorder
	loader   *probeLoader
}

// ワーカーは自分でトランザクションを commit するため、テストのトランザクションではなく、テンプレートから複製した DB で動かす。
// Hub を作ってテストの中だけの購読者に recorder の投影を登録し、DB の購読者・購読を合わせる（Start はしない）
func newHubFixture(t *testing.T, options eventdriven.Options) hubFixture {
	t.Helper()
	db, dsn := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	loader := newProbeLoader(eventstore.NewEventLoader(registry, operator.NewID, operator.NewEventType))
	hub, err := eventdriven.NewHub(testSubscribers, registry, options, loader)
	require.NoError(t, err)
	rec := &recorder{upserted: make(chan countingReadModel, 10)}
	require.NoError(t, eventdriven.RegisterProjection(hub, rec.spec()))
	require.NoError(t, hub.Sync(ctx))
	return hubFixture{ctx: ctx, db: db, dsn: dsn, registry: registry, hub: hub, recorder: rec, loader: loader}
}

func (f hubFixture) start(t *testing.T) {
	t.Helper()
	require.NoError(t, f.hub.Start(f.ctx, eventdriven.NewListener(f.dsn)))
	t.Cleanup(f.hub.Stop)
}

func (f hubFixture) store() domain.EventStore[operator.ID, operator.Event, operator.State] {
	return eventstore.New(f.registry, operator.NewID, operator.NewEventType, operator.InitialState, operator.Evolve)
}

// オペレーターを登録して commit する（知らせが Hub に届く）
func (f hubFixture) register(t *testing.T) operator.ID {
	t.Helper()
	return f.registerIn(t, f.ctx)
}

// ctx は f.ctx から作ったもの（トレースを載せた保存を試すため）
func (f hubFixture) registerIn(t *testing.T, ctx context.Context) operator.ID {
	t.Helper()
	id, err := rdb.Transact(ctx, func(ctx context.Context) (operator.ID, error) {
		id, publicID, err := identifier.New(f.registry, operator.NewID).Issue(ctx)
		if err != nil {
			return 0, err
		}
		events, err := operator.Decide(operator.InitialState(), &operator.RegisterCommand{
			ID: id, PublicID: publicID, Email: operatortest.Email(t, fmt.Sprintf("%s@example.com", uuid.NewString())),
		}, time.Now().UTC())
		if err != nil {
			return 0, err
		}
		return id, f.store().Save(ctx, events)
	})
	require.NoError(t, err)
	return id
}

func (f hubFixture) link(t *testing.T, id operator.ID) {
	t.Helper()
	_, err := rdb.Transact(f.ctx, func(ctx context.Context) (struct{}, error) {
		state, err := f.store().Load(ctx, id)
		if err != nil {
			return struct{}{}, err
		}
		events, err := operator.Decide(state, &operator.LinkCommand{
			Issuer: operatortest.Issuer(t, "diecardomain-studio"), Subject: operatortest.Subject(t, uuid.NewString()),
		}, time.Now().UTC())
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, f.store().Save(ctx, events)
	})
	require.NoError(t, err)
}

func (f hubFixture) checkpoint(t *testing.T, id operator.ID) (int64, bool) {
	t.Helper()
	var version int64
	err := f.db.QueryRowContext(f.ctx,
		`SELECT last_version FROM event_subscriber_checkpoints WHERE subscriber_id = 1 AND entity_id = $1`, id.Int64()).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false
	}
	require.NoError(t, err)
	return version, true
}

// Upsert は commit の前に呼ばれるため、read model を受け取った時点ではチェックポイントがまだ見えないことがある
func (f hubFixture) waitCheckpoint(t *testing.T, id operator.ID, want int64) {
	t.Helper()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		version, found := f.checkpoint(t, id)
		assert.True(c, found)
		assert.Equal(c, want, version)
	}, receiveTimeout, 10*time.Millisecond)
}

func TestHub_ProjectsCommittedEventOfSubscribedAggregate(t *testing.T) {
	f := newHubFixture(t, testOptions(2))
	f.start(t)

	id := f.register(t)

	require.Equal(t, countingReadModel{EntityID: id.Int64(), Applied: 1}, f.recorder.next(t))
	f.waitCheckpoint(t, id, 1)
}

// 投影の処理のログは、新しく反映したイベントごとに 1 行。保存のログと同じ項目の名前で、event_id で引ける
func TestHub_LogsProcessedEventOfProjection(t *testing.T) {
	records := logtest.Capture(t)
	f := newHubFixture(t, testOptions(2))
	f.start(t)

	id := f.register(t)

	f.recorder.next(t)
	rows := waitProcessedLogs(t, records, testProjectionName, id.Int64(), 1)
	row := rows[0]
	assert.Equal(t, slog.LevelInfo, row.Level)
	assert.Equal(t, "projection", row.Attrs["subscriber_kind"])
	assert.Equal(t, operator.Types().EntityType.String(), row.Attrs["entity_type"])
	assert.Equal(t, int64(1), row.Attrs["aggregate_version"])
	assert.Equal(t, "success", row.Attrs["result"])
	assert.NotZero(t, row.Attrs["event_id"])
	assert.NotEmpty(t, row.Attrs["event_type"])
	assert.GreaterOrEqual(t, row.Attrs["duration_ms"], int64(0))
	assert.GreaterOrEqual(t, row.Attrs["lag_ms"], int64(0))
	saved := records.Find("eventstore: event saved")
	require.NotEmpty(t, saved)
	assert.Equal(t, saved[len(saved)-1].Attrs["event_id"], row.Attrs["event_id"])
}

// 処理のログには、保存したときのトレースの trace_id・span_id が付く（スパンは作らず、保存したときの文脈をそのまま載せる）
func TestHub_LogsTraceOfSavedEvent(t *testing.T) {
	records := logtest.Capture(t)
	f := newHubFixture(t, testOptions(2))
	f.start(t)
	saved := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, SpanID: trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
	})

	id := f.registerIn(t, trace.ContextWithSpanContext(f.ctx, saved))

	f.recorder.next(t)
	row := waitProcessedLogs(t, records, testProjectionName, id.Int64(), 1)[0]
	assert.Equal(t, saved.TraceID().String(), row.Attrs["trace_id"])
	assert.Equal(t, saved.SpanID().String(), row.Attrs["span_id"])
}

// トレースを持たずに保存されたイベントの処理のログは、trace_id を持たない
func TestHub_LogsNoTraceForEventSavedWithoutTrace(t *testing.T) {
	records := logtest.Capture(t)
	f := newHubFixture(t, testOptions(2))
	f.start(t)

	id := f.register(t)

	f.recorder.next(t)
	row := waitProcessedLogs(t, records, testProjectionName, id.Int64(), 1)[0]
	assert.NotContains(t, row.Attrs, "trace_id")
	assert.NotContains(t, row.Attrs, "span_id")
}

// 作り直しで全イベントを当てても、処理のログは前に反映した版より後のイベントだけに出す
func TestHub_LogsOnlyNewEventsOfReprojection(t *testing.T) {
	records := logtest.Capture(t)
	f := newHubFixture(t, testOptions(2))
	f.start(t)
	id := f.register(t)
	f.recorder.next(t)
	waitProcessedLogs(t, records, testProjectionName, id.Int64(), 1)

	f.link(t, id)

	f.recorder.next(t)
	rows := waitProcessedLogs(t, records, testProjectionName, id.Int64(), 2)
	assert.Equal(t, int64(2), rows[1].Attrs["aggregate_version"])
}

func TestHub_RemovesPendingAfterProjecting(t *testing.T) {
	f := newHubFixture(t, testOptions(2))
	f.start(t)

	id := f.register(t)

	f.recorder.next(t)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		var count int
		assert.NoError(c, f.db.QueryRowContext(f.ctx,
			`SELECT count(*) FROM event_subscriber_pending WHERE subscriber_id = 1 AND entity_id = $1`, id.Int64()).Scan(&count))
		assert.Zero(c, count)
	}, receiveTimeout, 10*time.Millisecond)
}

func TestHub_ReprojectsFromAllEventsOnNextEvent(t *testing.T) {
	f := newHubFixture(t, testOptions(2))
	f.start(t)
	id := f.register(t)
	f.recorder.next(t)

	f.link(t, id)

	require.Equal(t, countingReadModel{EntityID: id.Int64(), Applied: 2}, f.recorder.next(t))
	f.waitCheckpoint(t, id, 2)
}

// 再接続や Poller で同じ知らせが重なって届いても、版が進んでいなければ書き直さない
func TestHub_SkipsAggregateAlreadyProjectedToLatestVersion(t *testing.T) {
	// ワーカーを 1 本にし、積んだ順に処理させる
	f := newHubFixture(t, testOptions(1))
	f.start(t)
	projected := f.register(t)
	f.recorder.next(t)

	f.hub.HandleEvent(eventdriven.EventRef{
		EntityTypeID: operatorEntityTypeID(t, f.registry), EntityID: projected.Int64(), AggregateVersion: 1,
	})
	other := f.register(t)

	// 先に積んだ projected が書かれていれば、other より先に届く
	require.Equal(t, other.Int64(), f.recorder.next(t).EntityID)
}

func TestHub_KeepsWorkingAfterProjectionPanics(t *testing.T) {
	// ワーカーを 1 本にし、panic した後も同じワーカーが次を処理することを確かめる
	f := newHubFixture(t, testOptions(1))
	f.recorder.panicAt = 2
	f.start(t)
	failing := f.register(t)
	f.recorder.next(t)
	f.waitCheckpoint(t, failing, 1)

	f.link(t, failing)
	other := f.register(t)

	require.Equal(t, other.Int64(), f.recorder.next(t).EntityID)
	// panic したトランザクションは rollback され、チェックポイントは進まない
	version, _ := f.checkpoint(t, failing)
	require.Equal(t, int64(1), version)
}

func TestHub_RejectsRegistrationAfterStart(t *testing.T) {
	f := newHubFixture(t, testOptions(1))
	f.start(t)
	spec := (&recorder{}).spec()

	err := eventdriven.RegisterProjection(f.hub, spec)

	require.ErrorContains(t, err, "after Start")
}

func TestHub_RejectsSecondStart(t *testing.T) {
	f := newHubFixture(t, testOptions(1))
	f.start(t)

	err := f.hub.Start(f.ctx, eventdriven.NewListener(f.dsn))

	require.Error(t, err)
}

// イベントの無い集約の知らせを、Listener を通さずに直接積む。commit しないため LISTEN の知らせは来ず、
// 積む順番をテストが決められる（イベントが無いので読み込むだけで、read model は書かない）
func (f hubFixture) notify(t *testing.T, entityID int64) {
	t.Helper()
	f.hub.HandleEvent(eventdriven.EventRef{EntityTypeID: operatorEntityTypeID(t, f.registry), EntityID: entityID, AggregateVersion: 1})
}

func TestHub_CoalescesNotificationsWhileWaiting(t *testing.T) {
	// ワーカーを 1 本にし、積んだ順に処理させる
	f := newHubFixture(t, testOptions(1))
	f.start(t)
	const blocker, target, firstMarker, secondMarker = 9001, 9002, 9003, 9004
	release := f.loader.holdNext(t, blocker, func() { f.notify(t, blocker) })

	f.notify(t, target)
	f.notify(t, target)
	f.notify(t, target)
	f.notify(t, firstMarker)
	release()
	f.loader.waitLoaded(t, firstMarker)
	// まとめていなければ、target の 2 回目は firstMarker の後に積まれ、secondMarker より先に読まれる
	f.notify(t, secondMarker)
	f.loader.waitLoaded(t, secondMarker)

	require.Equal(t, 1, f.loader.count(target))
}

// 処理中に読んだイベントには新しい版が入っていないかもしれないため、終わった後にもう 1 回だけ作り直す
func TestHub_ReprojectsOnceWhenNotifiedWhileProcessing(t *testing.T) {
	f := newHubFixture(t, testOptions(1))
	f.start(t)
	const target, firstMarker, secondMarker = 9001, 9002, 9003
	release := f.loader.holdNext(t, target, func() { f.notify(t, target) })

	f.notify(t, target)
	f.notify(t, target)
	f.notify(t, firstMarker)
	release()
	// target の作り直しは firstMarker の後に積まれる。2 回以上積まれていれば secondMarker より先に読まれる
	f.loader.waitLoaded(t, firstMarker)
	f.notify(t, secondMarker)
	f.loader.waitLoaded(t, secondMarker)

	require.Equal(t, 2, f.loader.count(target))
}

func operatorEntityTypeID(t *testing.T, registry *domaindef.Registry) domaindef.EntityTypeID {
	t.Helper()
	id, err := registry.EntityTypeID(operator.Types().EntityType)
	require.NoError(t, err)
	return id
}

// 知らせの無いところで入った行（Listener が動く前に commit されたイベント）を、起動したらすぐ拾う
func TestHub_ProjectsPendingAggregateOnStart(t *testing.T) {
	f := newHubFixture(t, testOptions(1))
	id := f.register(t)

	f.start(t)

	require.Equal(t, countingReadModel{EntityID: id.Int64(), Applied: 1}, f.recorder.next(t))
}

func TestHub_PollsPendingOnReconnect(t *testing.T) {
	f := newHubFixture(t, testOptions(1))
	const onStart, afterReconnect = 9001, 9002
	f.addPending(t, onStart)
	f.start(t)
	// 起動時に読んだ後で行を足す（起動時の読み込みに拾われないように）
	f.loader.waitLoaded(t, onStart)
	f.addPending(t, afterReconnect)

	f.hub.HandleReconnect()

	f.loader.waitLoaded(t, afterReconnect)
}

func TestHub_RetriesFailedProjectionByPolling(t *testing.T) {
	f := newHubFixture(t, eventdriven.Options{WorkerCount: 1, PollInterval: 50 * time.Millisecond, RetryInterval: time.Minute, MaxFailures: 10})
	f.recorder.panicAt = 1
	f.recorder.panicOnce = true
	// 起動時の読み込みを終えてから登録する（起動時の読み込みが、試し直しの代わりにならないように）
	const startMarker = 9001
	f.addPending(t, startMarker)
	f.start(t)
	f.loader.waitLoaded(t, startMarker)

	id := f.register(t)

	// 1 回目は panic で rollback され、やることの行が残るため、間隔で読んだときに作り直される
	require.Equal(t, countingReadModel{EntityID: id.Int64(), Applied: 1}, f.recorder.next(t))
	f.waitCheckpoint(t, id, 1)
}

// やることの表に、イベントの無い集約の行を直接入れる（知らせは来ない）
func (f hubFixture) addPending(t *testing.T, entityID int64) {
	t.Helper()
	_, err := f.db.ExecContext(f.ctx,
		`INSERT INTO event_subscriber_pending (subscriber_id, entity_type_id, entity_id, version) VALUES (1, 1000, $1, 1)`, entityID)
	require.NoError(t, err)
}

// README の「作り直す手順」の SQL を流すと、すでに作ってある集約も、すべてのイベントから作り直す
func TestHub_ReprojectsAllAggregatesByRebuildProcedure(t *testing.T) {
	f := newHubFixture(t, eventdriven.Options{WorkerCount: 1, PollInterval: 20 * time.Millisecond, RetryInterval: time.Minute, MaxFailures: 10})
	f.start(t)
	first := f.register(t)
	second := f.register(t)
	f.link(t, second)
	projected := map[int64]countingReadModel{}
	for len(projected) < 2 || projected[second.Int64()].Applied < 2 {
		state := f.recorder.next(t)
		projected[state.EntityID] = state
	}
	// recorder は read model を書く時点（commit の前）で受け取るため、やることの表が空になる（commit した）のを待ってから流す
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		var pending int
		assert.NoError(c, f.db.QueryRowContext(f.ctx, `SELECT count(*) FROM event_subscriber_pending`).Scan(&pending))
		assert.Zero(c, pending)
	}, receiveTimeout, 10*time.Millisecond)

	tx, err := f.db.BeginTx(f.ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(f.ctx, `DELETE FROM event_subscriber_checkpoints WHERE subscriber_id = $1`, testSubscribers[0].ID)
	require.NoError(t, err)
	_, err = tx.ExecContext(f.ctx, `
		INSERT INTO event_subscriber_pending (subscriber_id, entity_type_id, entity_id, version)
		SELECT $1, e.entity_type_id, e.entity_id, max(e.aggregate_version)
		FROM domain_events e
		JOIN event_subscriptions s ON s.subscriber_id = $1 AND s.entity_type_id = e.entity_type_id
		GROUP BY e.entity_type_id, e.entity_id
		ON CONFLICT (subscriber_id, entity_type_id, entity_id) DO UPDATE
		SET version = GREATEST(event_subscriber_pending.version, EXCLUDED.version)`, testSubscribers[0].ID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	rebuilt := map[int64]countingReadModel{}
	for len(rebuilt) < 2 {
		state := f.recorder.next(t)
		rebuilt[state.EntityID] = state
	}
	assert.Equal(t, countingReadModel{EntityID: first.Int64(), Applied: 1}, rebuilt[first.Int64()])
	assert.Equal(t, countingReadModel{EntityID: second.Int64(), Applied: 2}, rebuilt[second.Int64()])
}
