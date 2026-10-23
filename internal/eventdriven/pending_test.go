package eventdriven_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

type pendingRow struct {
	subscriberID int16
	version      int64
}

func pendingRows(t *testing.T, ctx context.Context, id operator.ID) []pendingRow {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	rows, err := exec.QueryContext(ctx,
		`SELECT subscriber_id, version FROM event_subscriber_pending WHERE entity_type_id = 1000 AND entity_id = $1 ORDER BY subscriber_id`, id.Int64())
	require.NoError(t, err)
	defer rows.Close()
	var result []pendingRow
	for rows.Next() {
		var row pendingRow
		require.NoError(t, rows.Scan(&row.subscriberID, &row.version))
		result = append(result, row)
	}
	require.NoError(t, rows.Err())
	return result
}

// operator を購読するのはテストの購読者（id 1）だけなので、行はその購読者の分だけ入る。
// 同じ集約のイベントが続いても行は増えず、版が上がる
func TestPendingTrigger_RecordsLatestVersionForSubscribersOfEntityType(t *testing.T) {
	ctx, hub := newHub(t)
	require.NoError(t, eventdriven.RegisterProjection(hub, (&writes{}).spec()))
	require.NoError(t, hub.Sync(ctx))
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	store := eventstore.New(registry, operator.NewID, operator.NewEventType, operator.InitialState, operator.Evolve)
	id, publicID, err := identifier.New(registry, operator.NewID).Issue(ctx)
	require.NoError(t, err)
	registered, err := operator.Decide(operator.InitialState(), &operator.RegisterCommand{
		ID: id, PublicID: publicID, Email: operatortest.Email(t, fmt.Sprintf("%s@example.com", uuid.NewString())),
	}, time.Now().UTC())
	require.NoError(t, err)

	require.NoError(t, store.Save(ctx, registered))
	require.Equal(t, []pendingRow{{subscriberID: 1, version: 1}}, pendingRows(t, ctx, id))

	state, err := store.Load(ctx, id)
	require.NoError(t, err)
	linked, err := operator.Decide(state, &operator.LinkCommand{
		Issuer: operatortest.Issuer(t, "diecardomain-studio"), Subject: operatortest.Subject(t, uuid.NewString()),
	}, time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, store.Save(ctx, linked))

	require.Equal(t, []pendingRow{{subscriberID: 1, version: 2}}, pendingRows(t, ctx, id))
}
