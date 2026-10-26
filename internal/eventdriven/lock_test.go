package eventdriven_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
)

// 同じ DB に、もう 1 台分の Hub（同じポリシーを登録）を作って動かす。core を 2 台動かしたときの形
func (f hubFixture) startAnotherPolicyHub(t *testing.T, options eventdriven.Options, policy *fakePolicy) {
	t.Helper()
	hub, err := eventdriven.NewHub(
		[]eventdriven.SubscriberDef{eventdriven.PolicySubscriber(policySubscriberID, testPolicyName)},
		f.registry, options, eventstore.NewEventLoader(f.registry, operator.NewID, operator.NewEventType),
	)
	require.NoError(t, err)
	require.NoError(t, eventdriven.RegisterPolicy(hub, policy.spec()))
	require.NoError(t, hub.Start(f.ctx, eventdriven.NewListener(f.dsn)))
	t.Cleanup(hub.Stop)
}

// 処理の単位のロックの番号（lock.go の決め方と同じ）。pg_locks は 64 ビットの番号を上位・下位の 32 ビットに分けて見せる
func unitLockID(id operator.ID) int64 {
	return int64(policySubscriberID)<<48 | id.Int64()
}

func (f hubFixture) advisoryLockCount(t *testing.T, id operator.ID) int {
	t.Helper()
	lockID := unitLockID(id)
	var count int
	require.NoError(t, f.db.QueryRowContext(f.ctx,
		`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND classid = $1 AND objid = $2 AND objsubid = 1`,
		lockID>>32, lockID&0xffffffff).Scan(&count))
	return count
}

// 2 台が同じ知らせを受けても、どのイベントもちょうど 1 回、集約ごとに版の順に処理される
func TestHub_HandlesEachEventOnceAcrossHubs(t *testing.T) {
	policy := newFakePolicy()
	options := eventdriven.Options{WorkerCount: 2, PollInterval: 20 * time.Millisecond, RetryInterval: time.Minute, MaxFailures: 10}
	f := newPolicyFixture(t, options, policy, nil)
	f.sync(t)
	f.start(t)
	f.startAnotherPolicyHub(t, options, policy)

	const operators = 10
	ids := make([]operator.ID, 0, operators)
	for range operators {
		id := f.register(t)
		f.link(t, id)
		ids = append(ids, id)
	}
	for _, id := range ids {
		f.waitPolicyCheckpoint(t, id, 2)
	}

	handled := map[int64][]int64{}
	for range operators * 2 {
		a := policy.next(t)
		handled[a.entityID] = append(handled[a.entityID], a.version)
	}
	for _, id := range ids {
		assert.Equal(t, []int64{1, 2}, handled[id.Int64()])
	}
	select {
	case extra := <-policy.attempts:
		t.Fatalf("handled more than once: %+v", extra)
	default:
	}
}

// 処理を終えたら、単位のロックを外す（残すと、その単位を誰も処理できなくなる）
func TestHub_ReleasesUnitLockAfterProcessing(t *testing.T) {
	policy := newFakePolicy()
	f := newPolicyFixture(t, policyOptions(time.Minute, 10), policy, nil)
	f.sync(t)
	f.start(t)

	id := f.register(t)
	policy.next(t)
	f.waitPendingRemoved(t, id)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Zero(c, f.advisoryLockCount(t, id))
	}, receiveTimeout, 10*time.Millisecond)
}

// ほかの台が処理中（ロックを持っている）の単位は飛ばし、その台が落ちて（接続が切れて）ロックが外れたら、続きを処理する
func TestHub_ProcessesUnitAfterHolderConnectionIsLost(t *testing.T) {
	policy := newFakePolicy()
	f := newPolicyFixture(t, policyOptions(time.Minute, 10), policy, nil)
	f.sync(t)
	id := f.register(t)
	holder, err := sql.Open("postgres", f.dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Close() })
	conn, err := holder.Conn(f.ctx)
	require.NoError(t, err)
	var acquired bool
	require.NoError(t, conn.QueryRowContext(f.ctx, `SELECT pg_try_advisory_lock($1)`, unitLockID(id)).Scan(&acquired))
	require.True(t, acquired)

	var holderPID int
	require.NoError(t, conn.QueryRowContext(f.ctx, `SELECT pg_backend_pid()`).Scan(&holderPID))
	var processingPID int
	require.NoError(t, f.db.QueryRowContext(f.ctx,
		`SELECT processing_pid FROM event_subscriber_pending_view WHERE subscriber_name = $1 AND entity_id = $2`,
		testPolicyName, id.Int64()).Scan(&processingPID))
	assert.Equal(t, holderPID, processingPID)

	f.start(t)
	// 起動時の Poller と何回かの間隔の分、ロックを持たれた単位を飛ばし続ける
	time.Sleep(100 * time.Millisecond)
	assert.Zero(t, f.policyCheckpoint(t, id))
	lost := time.Now()
	require.NoError(t, conn.Close())
	require.NoError(t, holder.Close())

	a := policy.next(t)
	assert.Equal(t, int64(1), a.version)
	assert.True(t, a.at.After(lost))
	f.waitPolicyCheckpoint(t, id, 1)
}
