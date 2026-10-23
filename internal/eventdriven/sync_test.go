package eventdriven_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

func queryStrings(t *testing.T, ctx context.Context, query string, args ...any) []string {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	rows, err := exec.QueryContext(ctx, query, args...)
	require.NoError(t, err)
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		require.NoError(t, rows.Scan(&value))
		values = append(values, value)
	}
	require.NoError(t, rows.Err())
	return values
}

func execSQL(t *testing.T, ctx context.Context, query string, args ...any) {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	_, err = exec.ExecContext(ctx, query, args...)
	require.NoError(t, err)
}

// operator を 1 人登録する（購読を足す前からある集約として使う）
func registerOperator(t *testing.T, ctx context.Context) operator.ID {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, publicID, err := identifier.New(registry, operator.NewID).Issue(ctx)
	require.NoError(t, err)
	registered, err := operator.Decide(operator.InitialState(), &operator.RegisterCommand{
		ID: id, PublicID: publicID, Email: operatortest.Email(t, fmt.Sprintf("%s@example.com", uuid.NewString())),
	}, time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, eventstore.New(registry, operator.NewID, operator.NewEventType, operator.InitialState, operator.Evolve).Save(ctx, registered))
	return id
}

// 購読者・購読を足し、購読を足す前からある集約にも、やることの行を入れる（入れないと次のイベントまで作られない）。2 回目は何も変えない
func TestHub_SyncAddsSubscriberSubscriptionAndPendingForExistingAggregates(t *testing.T) {
	ctx, hub := newHub(t)
	id := registerOperator(t, ctx)
	require.NoError(t, eventdriven.RegisterProjection(hub, (&writes{}).spec()))

	require.NoError(t, hub.Sync(ctx))
	require.NoError(t, hub.Sync(ctx))

	require.Equal(t, []string{"1: projection/" + testProjectionName}, queryStrings(t, ctx, `SELECT id || ': ' || kind || '/' || name FROM event_subscribers`))
	require.Equal(t, []string{"1: 1000"}, queryStrings(t, ctx, `SELECT subscriber_id || ': ' || entity_type_id FROM event_subscriptions`))
	require.Equal(t, []string{fmt.Sprintf("1: %d@1", id.Int64())},
		queryStrings(t, ctx, `SELECT subscriber_id || ': ' || entity_id || '@' || version FROM event_subscriber_pending`))
}

// 宣言から外した購読は、やること・チェックポイントと一緒に消し、新しく宣言した購読を足す
func TestHub_SyncRemovesSubscriptionNoLongerDeclared(t *testing.T) {
	ctx, hub := newHub(t)
	id := registerOperator(t, ctx)
	require.NoError(t, eventdriven.RegisterProjection(hub, (&writes{}).spec()))
	require.NoError(t, hub.Sync(ctx))
	execSQL(t, ctx, `INSERT INTO event_subscriber_checkpoints (subscriber_id, entity_type_id, entity_id, last_version, updated_at) VALUES (1, 1000, $1, 1, now())`, id.Int64())
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	next, err := eventdriven.NewHub(testSubscribers, registry, testOptions(1), eventstore.NewEventLoader(registry, creature.NewID, creature.NewEventType))
	require.NoError(t, err)
	spec := (&writes{}).spec()
	spec.Subscriptions = []domain.EntityType{creature.Types().EntityType}
	require.NoError(t, eventdriven.RegisterProjection(next, spec))

	require.NoError(t, next.Sync(ctx))

	require.Equal(t, []string{"1: 1100"}, queryStrings(t, ctx, `SELECT subscriber_id || ': ' || entity_type_id FROM event_subscriptions`))
	require.Empty(t, queryStrings(t, ctx, `SELECT entity_id::text FROM event_subscriber_pending WHERE entity_type_id = 1000`))
	require.Empty(t, queryStrings(t, ctx, `SELECT entity_id::text FROM event_subscriber_checkpoints WHERE entity_type_id = 1000`))
}

// DB に、Hub に渡していない購読者の行があれば止める（消さない。チェックポイント・やることも消えるため）
func TestHub_SyncRejectsSubscriberNotRegisteredInCode(t *testing.T) {
	ctx, hub := newHub(t)
	require.NoError(t, eventdriven.RegisterProjection(hub, (&writes{}).spec()))
	execSQL(t, ctx, `INSERT INTO event_subscribers (id, kind, name) VALUES (2, 'projection', 'removed_projection')`)

	err := hub.Sync(ctx)

	require.ErrorContains(t, err, "event_subscribers has a subscriber not registered in code: 2: projection/removed_projection")
	require.Equal(t, []string{"removed_projection"}, queryStrings(t, ctx, `SELECT name FROM event_subscribers WHERE id = 2`))
}

// 一覧で渡したのに、組み立てで登録し忘れた購読者は止める（DB の行だけができて、誰も処理しないため）
func TestHub_SyncRejectsSubscriberGivenButNotRegistered(t *testing.T) {
	ctx, hub := newHub(t)

	err := hub.Sync(ctx)

	require.ErrorContains(t, err, "subscriber is not registered: kind=projection, name="+testProjectionName)
}
