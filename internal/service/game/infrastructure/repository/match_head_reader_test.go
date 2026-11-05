package repository_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
)

// 型だけを持つイベントの行を足す（読み出しは最後の行の版・時刻・種類だけを見るので、中身は要らない）
func insertMatchEvent(t *testing.T, f playerFixture, id match.ID, version int64, name domain.EventName, at time.Time) {
	t.Helper()
	entityTypeID, err := f.registry.EntityTypeID(match.Types().EntityType)
	require.NoError(t, err)
	eventTypeID, err := f.registry.EventTypeID(entityTypeID, name)
	require.NoError(t, err)
	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	_, err = exec.ExecContext(f.ctx, `INSERT INTO domain_events (entity_type_id, entity_id, aggregate_version, event_type_id, event_type_version, payload, occurred_at)
		VALUES ($1, $2, $3, $4, 1, '{}', $5)`, entityTypeID.Int16(), id.Int64(), version, eventTypeID.Int16(), at)
	require.NoError(t, err)
}

func TestMatchHeadReader_ReadsTheLastEventOfTheMatch(t *testing.T) {
	f := newPlayerFixture(t)
	id, _ := issueEntity(t, f, match.NewID)
	other, _ := issueEntity(t, f, match.NewID)
	at := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	insertMatchEvent(t, f, id, 1, (&match.CreatedEvent{}).EventType(), at)
	insertMatchEvent(t, f, id, 2, (&match.TurnEndedEvent{}).EventType(), at.Add(time.Second))
	insertMatchEvent(t, f, other, 1, (&match.MatchEndedEvent{}).EventType(), at.Add(time.Hour))

	head, err := repository.NewMatchHeadReader().Load(f.ctx, id)

	require.NoError(t, err)
	require.Equal(t, int64(2), head.Revision)
	require.True(t, at.Add(time.Second).Equal(head.OccurredAt))
	require.False(t, head.Ended)
}

func TestMatchHeadReader_ReportsEndedWhenTheLastEventIsMatchEnded(t *testing.T) {
	f := newPlayerFixture(t)
	id, _ := issueEntity(t, f, match.NewID)
	at := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	insertMatchEvent(t, f, id, 1, (&match.CreatedEvent{}).EventType(), at)
	insertMatchEvent(t, f, id, 2, (&match.MatchEndedEvent{}).EventType(), at.Add(time.Second))

	head, err := repository.NewMatchHeadReader().Load(f.ctx, id)

	require.NoError(t, err)
	require.Equal(t, int64(2), head.Revision)
	require.True(t, head.Ended)
}

func TestMatchHeadReader_FailsForAMatchWithoutEvents(t *testing.T) {
	f := newPlayerFixture(t)
	id, _ := issueEntity(t, f, match.NewID)

	_, err := repository.NewMatchHeadReader().Load(f.ctx, id)

	require.Error(t, err)
}
