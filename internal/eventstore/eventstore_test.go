package eventstore_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/logtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

func TestSaveAndLoad_RestoresStateFromEvents(t *testing.T) {
	ctx, registry := setup(t)
	occurredAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	id, publicID, events := register(t, ctx, registry, "operator@example.com", occurredAt)
	store := operatorStore(registry)

	require.NoError(t, store.Save(ctx, events))
	state, err := store.Load(ctx, id)

	require.NoError(t, err)
	require.Equal(t, operator.Operator{
		ID: id, Version: 1, PublicID: publicID,
		Email: operatortest.Email(t, "operator@example.com"),
		// created_at には DB の現在時刻ではなく発生時刻が残る
		RegisteredAt: occurredAt,
	}, normalize(state))
}

// 保存のログは、トランザクションが確定したあとに、イベントごとに 1 行出る
func TestSave_LogsEventAfterCommit(t *testing.T) {
	records := logtest.Capture(t)
	ctx, registry := isolatedSetup(t)
	occurredAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	id, _, events := register(t, ctx, registry, "operator@example.com", occurredAt)

	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		err := operatorStore(registry).Save(ctx, events)
		require.Empty(t, records.Find("eventstore: event saved"))
		return struct{}{}, err
	})

	require.NoError(t, err)
	rows := records.Find("eventstore: event saved")
	require.Len(t, rows, 1)
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var eventID int64
	require.NoError(t, exec.QueryRowContext(ctx, `SELECT id FROM domain_events WHERE entity_id = $1`, id.Int64()).Scan(&eventID))
	require.Equal(t, slog.LevelInfo, rows[0].Level)
	require.Equal(t, map[string]any{
		"event_id": eventID, "entity_type": operator.Types().EntityType.String(), "entity_id": id.Int64(),
		"aggregate_version": int64(1), "event_type": events[0].EventType().String(), "occurred_at": occurredAt,
	}, rows[0].Attrs)
}

// 取り消された保存は記録に残さない
func TestSave_DoesNotLogEventWhenTransactionRollsBack(t *testing.T) {
	records := logtest.Capture(t)
	ctx, registry := isolatedSetup(t)
	_, _, events := register(t, ctx, registry, "operator@example.com", time.Now().UTC())

	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		require.NoError(t, operatorStore(registry).Save(ctx, events))
		return struct{}{}, errors.New("rollback")
	})

	require.Error(t, err)
	require.Empty(t, records.Find("eventstore: event saved"))
}

func TestSave_RejectsSameVersionTwice(t *testing.T) {
	ctx, registry := setup(t)
	_, _, events := register(t, ctx, registry, "operator@example.com", time.Now().UTC())
	store := operatorStore(registry)
	require.NoError(t, store.Save(ctx, events))

	err := store.Save(ctx, events)

	require.ErrorIs(t, err, domain.ErrVersionConflict)
	// 衝突してもトランザクションは中断されず、続けて使える（DB にエラーのログも出ない）
	_, err = store.Load(ctx, events[0].GetMeta().EntityID)
	require.NoError(t, err)
}

// 版を変えた同じ内容のイベントを作る（保存の版の決まりだけを確かめるため、中身は問わない）
func withVersion(t *testing.T, event operator.Event, version int64) operator.Event {
	t.Helper()
	registered := *event.(*operator.RegisteredEvent)
	registered.AggregateVersion = version
	return &registered
}

func TestSave_RejectsNonConsecutiveVersion(t *testing.T) {
	tests := []struct {
		name         string
		saveVersion1 bool
		version      int64
	}{
		{name: "skips_versions", saveVersion1: true, version: 3},
		{name: "first_event_is_not_version_1", saveVersion1: false, version: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, registry := setup(t)
			_, _, events := register(t, ctx, registry, "non-consecutive-"+uuid.NewString()+"@example.com", time.Now().UTC())
			store := operatorStore(registry)
			if tt.saveVersion1 {
				require.NoError(t, store.Save(ctx, events))
			}

			err := store.Save(ctx, []operator.Event{withVersion(t, events[0], tt.version)})

			require.ErrorIs(t, err, domain.ErrVersionNotConsecutive)
			require.NotErrorIs(t, err, domain.ErrVersionConflict)
		})
	}
}

// 1 回の保存で渡した、続いた版のイベントは保存できる（前のイベントが同じ保存の中で入る）
func TestSave_AcceptsConsecutiveVersionsInOneSave(t *testing.T) {
	ctx, registry := setup(t)
	id, _, events := register(t, ctx, registry, "consecutive-"+uuid.NewString()+"@example.com", time.Now().UTC())
	store := operatorStore(registry)

	err := store.Save(ctx, []operator.Event{events[0], withVersion(t, events[0], 2), withVersion(t, events[0], 3)})

	require.NoError(t, err)
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var count int
	require.NoError(t, exec.QueryRowContext(ctx,
		`SELECT count(*) FROM domain_events WHERE entity_type_id = (SELECT id FROM domain_entity_types WHERE name = 'operator') AND entity_id = $1`, id.Int64()).Scan(&count))
	require.Equal(t, 3, count)
}

func TestSave_StoresEventTypeVersion(t *testing.T) {
	ctx, registry := setup(t)
	id, _, events := register(t, ctx, registry, "operator@example.com", time.Now().UTC())
	require.NoError(t, operatorStore(registry).Save(ctx, events))

	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var version int
	require.NoError(t, exec.QueryRowContext(ctx,
		`SELECT event_type_version FROM domain_events WHERE entity_id = $1`, id.Int64()).Scan(&version))
	require.Equal(t, events[0].SchemaVersion(), version)
}

func TestLoad_ReturnsInitialStateWhenNoEvents(t *testing.T) {
	ctx, registry := setup(t)
	id, _, err := identifier.New(registry, operator.NewID).Issue(ctx)
	require.NoError(t, err)

	state, err := operatorStore(registry).Load(ctx, id)

	require.NoError(t, err)
	require.Equal(t, operator.InitialState(), state)
}

// entity_id は集約の種別をまたいで重なりうるため、Load はほかの種別のイベントを読まない
func TestLoad_IgnoresEventsOfOtherEntityTypes(t *testing.T) {
	ctx, registry := isolatedSetup(t)
	id, _, events := register(t, ctx, registry, "operator@example.com", time.Now().UTC())
	require.NoError(t, operatorStore(registry).Save(ctx, events))
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	// テストの中だけの種別に、同じ entity_id・次の版のイベントを入れる
	_, err = exec.ExecContext(ctx, `INSERT INTO domain_entity_types (id, name) VALUES (1, 'eventstore_test')`)
	require.NoError(t, err)
	_, err = exec.ExecContext(ctx, `INSERT INTO domain_event_types (id, entity_type_id, name) VALUES (1, 1, 'RegisteredEvent')`)
	require.NoError(t, err)
	_, err = exec.ExecContext(ctx, `
		INSERT INTO domain_events (entity_type_id, entity_id, aggregate_version, event_type_id, event_type_version, payload, created_at)
		VALUES (1, $1, 2, 1, 1, '{}', now())`, id.Int64())
	require.NoError(t, err)

	state, err := operatorStore(registry).Load(ctx, id)

	require.NoError(t, err)
	require.Equal(t, int64(1), state.(operator.Operator).Version)
}

func TestIdentifier_IssuesDistinctIDsAndResolvesPublicID(t *testing.T) {
	ctx, registry := setup(t)
	issuer := identifier.New(registry, operator.NewID)
	id1, publicID1, err := issuer.Issue(ctx)
	require.NoError(t, err)
	id2, publicID2, err := issuer.Issue(ctx)
	require.NoError(t, err)

	require.NotEqual(t, id1, id2)
	require.NotEqual(t, publicID1, publicID2)
	resolved, err := issuer.Resolve(ctx, publicID2)
	require.NoError(t, err)
	require.Equal(t, id2, resolved)
}

func TestIdentifier_ResolveFailsForUnknownPublicID(t *testing.T) {
	ctx, registry := setup(t)

	_, err := identifier.New(registry, operator.NewID).Resolve(ctx, uuid.New())

	require.ErrorIs(t, err, domain.ErrEntityNotFound)
}

func TestIdentifier_ResolveFailsForPublicIDOfOtherEntityType(t *testing.T) {
	ctx, registry := setup(t)
	_, publicID, err := identifier.New(registry, creature.NewID).Issue(ctx)
	require.NoError(t, err)

	_, err = identifier.New(registry, operator.NewID).Resolve(ctx, publicID)

	require.ErrorIs(t, err, domain.ErrEntityNotFound)
}

func TestIdentifier_DeleteReleasesUniqueKeysAndKeepsEvents(t *testing.T) {
	ctx, registry := setup(t)
	// 同時に動くほかのテストのオペレーターとぶつからない値にする
	email := "delete-" + uuid.NewString() + "@example.com"
	id, publicID, events := register(t, ctx, registry, email, time.Now().UTC())
	require.NoError(t, operatorStore(registry).Save(ctx, events))
	require.NoError(t, constraint.UpdateOperatorEmail(ctx, id, operatortest.Email(t, email)))
	issuer := identifier.New(registry, operator.NewID)

	require.NoError(t, issuer.Delete(ctx, id))

	_, err := issuer.Resolve(ctx, publicID)
	require.ErrorIs(t, err, domain.ErrEntityNotFound)
	other, _, err := issuer.Issue(ctx)
	require.NoError(t, err)
	require.NoError(t, constraint.UpdateOperatorEmail(ctx, other, operatortest.Email(t, email)))
	state, err := operatorStore(registry).Load(ctx, id)
	require.NoError(t, err)
	require.Equal(t, int64(1), state.(operator.Operator).Version)
}

func TestEntityRefResolver_ResolvesEntityTypeAndID(t *testing.T) {
	ctx, registry := setup(t)
	id, publicID, err := identifier.New(registry, creature.NewID).Issue(ctx)
	require.NoError(t, err)

	ref, err := identifier.NewEntityRefResolver(registry).Resolve(ctx, publicID)

	require.NoError(t, err)
	require.Equal(t, domain.NewEntityRef(creature.Types().EntityType, id.Int64()), ref)
}

func TestEntityRefResolver_ResolveFailsForUnknownPublicID(t *testing.T) {
	ctx, registry := setup(t)

	_, err := identifier.NewEntityRefResolver(registry).Resolve(ctx, uuid.New())

	require.ErrorIs(t, err, domain.ErrEntityNotFound)
}

// 無い公開 ID は結果に含めない
func TestEntityRefResolver_ResolvesAllKnownPublicIDs(t *testing.T) {
	ctx, registry := setup(t)
	creatureID, creaturePublicID, err := identifier.New(registry, creature.NewID).Issue(ctx)
	require.NoError(t, err)
	itemID, itemPublicID, err := identifier.New(registry, item.NewID).Issue(ctx)
	require.NoError(t, err)

	refs, err := identifier.NewEntityRefResolver(registry).ResolveAll(ctx, []uuid.UUID{creaturePublicID, itemPublicID, uuid.New()})

	require.NoError(t, err)
	require.Equal(t, map[uuid.UUID]domain.EntityRef{
		creaturePublicID: domain.NewEntityRef(creature.Types().EntityType, creatureID.Int64()),
		itemPublicID:     domain.NewEntityRef(item.Types().EntityType, itemID.Int64()),
	}, refs)
}

func TestEntityRefResolver_ReturnsPublicIDsOfKnownRefs(t *testing.T) {
	ctx, registry := setup(t)
	creatureID, creaturePublicID, err := identifier.New(registry, creature.NewID).Issue(ctx)
	require.NoError(t, err)
	itemID, itemPublicID, err := identifier.New(registry, item.NewID).Issue(ctx)
	require.NoError(t, err)
	creatureRef := domain.NewEntityRef(creature.Types().EntityType, creatureID.Int64())
	itemRef := domain.NewEntityRef(item.Types().EntityType, itemID.Int64())
	// 内部 ID は合っていても種類が違えば、別の集約の公開 ID を返さない
	wrongTypeRef := domain.NewEntityRef(spell.Types().EntityType, creatureID.Int64())
	unknownRef := domain.NewEntityRef(creature.Types().EntityType, itemID.Int64()+1000)

	publicIDs, err := identifier.NewEntityRefResolver(registry).PublicIDs(ctx, []domain.EntityRef{creatureRef, itemRef, wrongTypeRef, unknownRef})

	require.NoError(t, err)
	require.Equal(t, map[domain.EntityRef]uuid.UUID{creatureRef: creaturePublicID, itemRef: itemPublicID}, publicIDs)
}

// DB から読んだ時刻は Location が UTC と異なりうるため、比較のために UTC にそろえる
func normalize(state operator.State) operator.State {
	if op, ok := state.(operator.Operator); ok {
		op.RegisteredAt = op.RegisteredAt.UTC()
		return op
	}
	return state
}
