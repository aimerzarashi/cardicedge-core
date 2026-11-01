package eventdriven_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/logtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

const (
	testPolicyName     = "eventdriven_test_policy"
	policySubscriberID = 2
)

// ポリシーが受け取った 1 回分の呼び出し。checkpoint は、受け取った時点のチェックポイント
type attempt struct {
	entityID   int64
	version    int64
	checkpoint int64
	at         time.Time
	failed     bool
	// Handle の ctx に載っていたトレース（無ければ無効な SpanContext）
	span trace.SpanContext
}

// 受け取ったイベントを覚えるだけの、テストの中だけのポリシー（operator を購読する）。
// failAt で指した集約の、指した版以降のイベントを、指した回数だけ失敗させる
type fakePolicy struct {
	attempts      chan attempt
	failEntity    atomic.Int64
	failFrom      atomic.Int64
	failRemaining atomic.Int32
}

func newFakePolicy() *fakePolicy {
	return &fakePolicy{attempts: make(chan attempt, 100)}
}

const failForever = 1 << 30

func (p *fakePolicy) failAt(id operator.ID, fromVersion int64, times int32) {
	p.failEntity.Store(id.Int64())
	p.failFrom.Store(fromVersion)
	p.failRemaining.Store(times)
}

func (p *fakePolicy) spec() eventdriven.PolicySpec {
	return eventdriven.PolicySpec{
		Name:          testPolicyName,
		Subscriptions: []domain.EntityType{operator.Types().EntityType},
		Handle: func(ctx context.Context, event eventstore.LoadedEvent) error {
			entityID := event.Event.(operator.Event).GetMeta().EntityID.Int64()
			exec, err := rdb.GetExecutor(ctx)
			if err != nil {
				return err
			}
			var checkpoint int64
			if err := exec.QueryRowContext(ctx,
				`SELECT COALESCE(max(last_version), 0) FROM event_subscriber_checkpoints WHERE subscriber_id = $1 AND entity_id = $2`,
				policySubscriberID, entityID).Scan(&checkpoint); err != nil {
				return err
			}
			failed := p.failEntity.Load() == entityID && event.AggregateVersion >= p.failFrom.Load() && p.failRemaining.Add(-1) >= 0
			p.attempts <- attempt{entityID: entityID, version: event.AggregateVersion, checkpoint: checkpoint, at: time.Now(), failed: failed, span: trace.SpanContextFromContext(ctx)}
			if failed {
				return errors.New("eventdriven test failure")
			}
			return nil
		},
	}
}

func (p *fakePolicy) next(t *testing.T) attempt {
	t.Helper()
	select {
	case a := <-p.attempts:
		return a
	case <-time.After(receiveTimeout):
		t.Fatal("no policy event received")
		return attempt{}
	}
}

// ポリシー（と、withProjection なら投影）を登録した Hub を作る。Sync と Start はしない
// （購読を足す前にイベントを保存するテストがあるため）
func newPolicyFixture(t *testing.T, options eventdriven.Options, policy *fakePolicy, projection *recorder) hubFixture {
	t.Helper()
	db, dsn := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	subscribers := []eventdriven.SubscriberDef{eventdriven.PolicySubscriber(policySubscriberID, testPolicyName)}
	if projection != nil {
		subscribers = append(subscribers, testSubscribers...)
	}
	hub, err := eventdriven.NewHub(subscribers, registry, options, eventstore.NewEventLoader(registry, operator.NewID, operator.NewEventType))
	require.NoError(t, err)
	require.NoError(t, eventdriven.RegisterPolicy(hub, policy.spec()))
	if projection != nil {
		require.NoError(t, eventdriven.RegisterProjection(hub, projection.spec()))
	}
	return hubFixture{ctx: ctx, db: db, dsn: dsn, registry: registry, hub: hub}
}

func policyOptions(retryInterval time.Duration, maxFailures int) eventdriven.Options {
	return eventdriven.Options{WorkerCount: 1, PollInterval: 20 * time.Millisecond, RetryInterval: retryInterval, MaxFailures: maxFailures}
}

func (f hubFixture) sync(t *testing.T) {
	t.Helper()
	require.NoError(t, f.hub.Sync(f.ctx))
}

func (f hubFixture) policyCheckpoint(t *testing.T, id operator.ID) int64 {
	t.Helper()
	var version int64
	require.NoError(t, f.db.QueryRowContext(f.ctx,
		`SELECT COALESCE(max(last_version), 0) FROM event_subscriber_checkpoints WHERE subscriber_id = $1 AND entity_id = $2`,
		policySubscriberID, id.Int64()).Scan(&version))
	return version
}

func (f hubFixture) failureCount(t *testing.T, id operator.ID) int {
	t.Helper()
	var count int
	require.NoError(t, f.db.QueryRowContext(f.ctx,
		`SELECT COALESCE(max(failure_count), 0) FROM event_subscriber_failures WHERE subscriber_id = $1 AND entity_id = $2`,
		policySubscriberID, id.Int64()).Scan(&count))
	return count
}

func (f hubFixture) pendingCount(t *testing.T, id operator.ID) int {
	t.Helper()
	var count int
	require.NoError(t, f.db.QueryRowContext(f.ctx,
		`SELECT count(*) FROM event_subscriber_pending WHERE subscriber_id = $1 AND entity_id = $2`,
		policySubscriberID, id.Int64()).Scan(&count))
	return count
}

func (f hubFixture) waitPendingRemoved(t *testing.T, id operator.ID) {
	t.Helper()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Zero(c, f.pendingCount(t, id))
	}, receiveTimeout, 10*time.Millisecond)
}

func (f hubFixture) waitFailureCount(t *testing.T, id operator.ID, want int) {
	t.Helper()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, want, f.failureCount(t, id))
	}, receiveTimeout, 10*time.Millisecond)
}

func (f hubFixture) waitPolicyCheckpoint(t *testing.T, id operator.ID, want int64) {
	t.Helper()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, want, f.policyCheckpoint(t, id))
	}, receiveTimeout, 10*time.Millisecond)
}

// 止まっている間にたまった 2 つのイベントが、版の順に 1 つずつ届き、チェックポイントがイベントごとに進む
func TestPolicy_HandlesEventsOneByOneInVersionOrder(t *testing.T) {
	policy := newFakePolicy()
	f := newPolicyFixture(t, policyOptions(time.Minute, 10), policy, nil)
	f.sync(t)
	id := f.register(t)
	f.link(t, id)

	f.start(t)

	assert.Equal(t, attempt{entityID: id.Int64(), version: 1, checkpoint: 0}, withoutTime(policy.next(t)))
	assert.Equal(t, attempt{entityID: id.Int64(), version: 2, checkpoint: 1}, withoutTime(policy.next(t)))
	f.waitPolicyCheckpoint(t, id, 2)
	f.waitPendingRemoved(t, id)
}

// ポリシーの処理のログは、イベントごとに 1 行。失敗したイベントには result が failure の行が出る
func TestPolicy_LogsProcessedEventWithResult(t *testing.T) {
	records := logtest.Capture(t)
	policy := newFakePolicy()
	f := newPolicyFixture(t, policyOptions(time.Minute, 10), policy, nil)
	f.sync(t)
	id := f.register(t)
	f.link(t, id)
	policy.failAt(id, 2, failForever)

	f.start(t)

	rows := waitProcessedLogs(t, records, testPolicyName, id.Int64(), 2)
	assert.Equal(t, "policy", rows[0].Attrs["subscriber_kind"])
	assert.Equal(t, int64(1), rows[0].Attrs["aggregate_version"])
	assert.Equal(t, "success", rows[0].Attrs["result"])
	assert.Equal(t, int64(2), rows[1].Attrs["aggregate_version"])
	assert.Equal(t, "failure", rows[1].Attrs["result"])
	assert.GreaterOrEqual(t, rows[1].Attrs["duration_ms"], int64(0))
	assert.GreaterOrEqual(t, rows[1].Attrs["lag_ms"], int64(0))
}

func savedSpan() trace.SpanContext {
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, SpanID: trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
	})
}

// ポリシーのコマンドが保存するイベントにも同じ trace_id が入るよう、保存したときのトレースを ctx に載せて渡す
func TestPolicy_CarriesTraceOfSavedEventToHandler(t *testing.T) {
	policy := newFakePolicy()
	f := newPolicyFixture(t, policyOptions(time.Minute, 10), policy, nil)
	f.sync(t)
	saved := savedSpan()
	f.registerIn(t, trace.ContextWithSpanContext(f.ctx, saved))

	f.start(t)

	assert.Equal(t, saved.TraceID(), policy.next(t).span.TraceID())
}

func TestPolicy_CarriesNoTraceForEventSavedWithoutTrace(t *testing.T) {
	policy := newFakePolicy()
	f := newPolicyFixture(t, policyOptions(time.Minute, 10), policy, nil)
	f.sync(t)
	f.register(t)

	f.start(t)

	assert.False(t, policy.next(t).span.IsValid())
}

// 失敗のログにも保存したときの trace_id が付く
func TestPolicy_LogsFailureWithTraceOfSavedEvent(t *testing.T) {
	records := logtest.Capture(t)
	policy := newFakePolicy()
	f := newPolicyFixture(t, policyOptions(time.Minute, 10), policy, nil)
	f.sync(t)
	saved := savedSpan()
	id := f.registerIn(t, trace.ContextWithSpanContext(f.ctx, saved))
	policy.failAt(id, 1, failForever)

	f.start(t)

	require.True(t, policy.next(t).failed)
	f.waitFailureCount(t, id, 1)
	failed := records.Find("eventdriven: policy failed")
	require.NotEmpty(t, failed)
	assert.Equal(t, saved.TraceID().String(), failed[0].Attrs["trace_id"])
}

func withoutTime(a attempt) attempt {
	a.at = time.Time{}
	a.span = trace.SpanContext{}
	return a
}

// 失敗したイベントで止め、間隔が過ぎるまでやり直さない。過ぎたら、済んだイベントをやり直さずに続きから進む
func TestPolicy_RetriesFailedEventAfterInterval(t *testing.T) {
	const retryInterval = 500 * time.Millisecond
	policy := newFakePolicy()
	f := newPolicyFixture(t, policyOptions(retryInterval, 10), policy, nil)
	f.sync(t)
	id := f.register(t)
	f.link(t, id)
	policy.failAt(id, 2, 1)

	f.start(t)

	assert.Equal(t, int64(1), policy.next(t).version)
	failed := policy.next(t)
	require.True(t, failed.failed)
	require.Equal(t, int64(2), failed.version)
	f.waitFailureCount(t, id, 1)
	retried := policy.next(t)
	assert.Equal(t, int64(2), retried.version)
	assert.False(t, retried.failed)
	assert.GreaterOrEqual(t, retried.at.Sub(failed.at), retryInterval)
	f.waitPolicyCheckpoint(t, id, 2)
	f.waitFailureCount(t, id, 0)
	f.waitPendingRemoved(t, id)
}

// 失敗したイベントより後のイベントは、失敗が直るまで処理しない
func TestPolicy_StopsAtFailedEvent(t *testing.T) {
	policy := newFakePolicy()
	f := newPolicyFixture(t, policyOptions(time.Hour, 10), policy, nil)
	f.sync(t)
	id := f.register(t)
	f.link(t, id)
	policy.failAt(id, 1, failForever)

	f.start(t)

	require.True(t, policy.next(t).failed)
	f.waitFailureCount(t, id, 1)
	// 後ろの集約が処理されたとき、前のキーの処理は終わっている
	other := f.register(t)
	require.Equal(t, other.Int64(), policy.next(t).entityID)
	require.Empty(t, policy.attempts)
	require.Zero(t, f.policyCheckpoint(t, id))
}

// 回数の上限に達したら、デッドレターとしてそれ以上やり直さない
func TestPolicy_StopsRetryingAtMaxFailures(t *testing.T) {
	policy := newFakePolicy()
	f := newPolicyFixture(t, policyOptions(10*time.Millisecond, 2), policy, nil)
	f.sync(t)
	id := f.register(t)
	policy.failAt(id, 1, failForever)

	f.start(t)

	f.waitFailureCount(t, id, 2)
	f.link(t, id)
	other := f.register(t)
	// 先に積んだ id のキーは、other より先に処理される（ワーカーは 1 本）
	for {
		a := policy.next(t)
		if a.entityID == other.Int64() {
			break
		}
	}
	require.Equal(t, 2, f.failureCount(t, id))
	require.Empty(t, policy.attempts)
}

// 失敗の記録の行を消す（デッドレターを外す）と、続きから進む
func TestPolicy_ResumesWhenFailureRecordIsRemoved(t *testing.T) {
	policy := newFakePolicy()
	f := newPolicyFixture(t, policyOptions(10*time.Millisecond, 2), policy, nil)
	f.sync(t)
	id := f.register(t)
	policy.failAt(id, 1, failForever)
	f.start(t)
	f.waitFailureCount(t, id, 2)
	policy.failRemaining.Store(0)

	_, err := f.db.ExecContext(f.ctx, `DELETE FROM event_subscriber_failures WHERE subscriber_id = $1 AND entity_id = $2`, policySubscriberID, id.Int64())
	require.NoError(t, err)

	f.waitPolicyCheckpoint(t, id, 1)
	require.Zero(t, f.failureCount(t, id))
}

// 購読を足す前に保存されたイベントでは動かず、足した後のイベントでだけ動く
func TestPolicy_HandlesOnlyEventsAfterSubscriptionAdded(t *testing.T) {
	policy := newFakePolicy()
	f := newPolicyFixture(t, policyOptions(time.Minute, 10), policy, nil)
	existing := f.register(t)
	f.sync(t)
	require.Equal(t, int64(1), f.policyCheckpoint(t, existing))
	require.Zero(t, f.pendingCount(t, existing))
	f.start(t)

	f.link(t, existing)
	added := f.register(t)

	assert.Equal(t, attempt{entityID: existing.Int64(), version: 2, checkpoint: 1}, withoutTime(policy.next(t)))
	assert.Equal(t, attempt{entityID: added.Int64(), version: 1, checkpoint: 0}, withoutTime(policy.next(t)))
}

// 購読を消すと、失敗の記録も消える
func TestHub_SyncRemovesFailuresOfRemovedSubscription(t *testing.T) {
	policy := newFakePolicy()
	f := newPolicyFixture(t, policyOptions(time.Minute, 10), policy, nil)
	f.sync(t)
	id := f.register(t)
	_, err := f.db.ExecContext(f.ctx,
		`INSERT INTO event_subscriber_failures (subscriber_id, entity_type_id, entity_id, failure_count, last_failed_at) VALUES ($1, 1000, $2, 3, now())`,
		policySubscriberID, id.Int64())
	require.NoError(t, err)
	next, err := eventdriven.NewHub([]eventdriven.SubscriberDef{eventdriven.PolicySubscriber(policySubscriberID, testPolicyName)}, f.registry,
		policyOptions(time.Minute, 10), eventstore.NewEventLoader(f.registry, creature.NewID, creature.NewEventType))
	require.NoError(t, err)
	spec := policy.spec()
	spec.Subscriptions = []domain.EntityType{creature.Types().EntityType}
	require.NoError(t, eventdriven.RegisterPolicy(next, spec))

	require.NoError(t, next.Sync(f.ctx))

	require.Zero(t, f.failureCount(t, id))
}

// 投影とポリシーを同じ Hub に登録しても、それぞれ動く
func TestPolicy_RunsAlongsideProjection(t *testing.T) {
	policy := newFakePolicy()
	projection := &recorder{upserted: make(chan countingReadModel, 10)}
	f := newPolicyFixture(t, policyOptions(time.Minute, 10), policy, projection)
	f.sync(t)
	f.start(t)

	id := f.register(t)

	assert.Equal(t, countingReadModel{EntityID: id.Int64(), Applied: 1}, projection.next(t))
	assert.Equal(t, attempt{entityID: id.Int64(), version: 1, checkpoint: 0}, withoutTime(policy.next(t)))
}

func TestRegisterPolicy_RejectsInvalidSpec(t *testing.T) {
	_, hub := newHub(t)

	err := eventdriven.RegisterPolicy(hub, eventdriven.PolicySpec{})

	require.ErrorContains(t, err, "name is required")
	require.ErrorContains(t, err, "subscriptions is required")
	require.ErrorContains(t, err, "handle is required")
}

func TestNewHub_RejectsInvalidPolicyOptions(t *testing.T) {
	tests := map[string]eventdriven.Options{
		"zero_retry_interval": {WorkerCount: 1, PollInterval: time.Second, MaxFailures: 10},
		"zero_max_failures":   {WorkerCount: 1, PollInterval: time.Second, RetryInterval: time.Minute},
	}
	for name, options := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := eventdriven.NewHub(nil, nil, options)

			require.Error(t, err)
		})
	}
}

// 途中まで処理して止まった後、別の Hub（再起動した core）が続きから処理する。処理済みのイベントは渡し直さず、
// 止まっている間に来たイベントは版の順に渡す
func TestPolicy_ResumesAfterRestartWithoutDuplicatesOrGaps(t *testing.T) {
	policy := newFakePolicy()
	options := policyOptions(time.Minute, 10)
	f := newPolicyFixture(t, options, policy, nil)
	f.sync(t)
	f.start(t)
	id := f.register(t)
	assert.Equal(t, attempt{entityID: id.Int64(), version: 1, checkpoint: 0}, withoutTime(policy.next(t)))
	f.waitPolicyCheckpoint(t, id, 1)

	f.hub.Stop()
	f.link(t, id)
	f.startAnotherPolicyHub(t, options, policy)

	assert.Equal(t, attempt{entityID: id.Int64(), version: 2, checkpoint: 1}, withoutTime(policy.next(t)))
	f.waitPolicyCheckpoint(t, id, 2)
	f.waitPendingRemoved(t, id)
	select {
	case extra := <-policy.attempts:
		t.Fatalf("handled more than once: %+v", extra)
	default:
	}
}
