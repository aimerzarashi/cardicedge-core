package eventdriven_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

// 通知は非同期に届くため、届くのを待つ上限。届かないことの確認には使わない（待つだけ遅くなるため）
const receiveTimeout = 5 * time.Second

type recordingHandler struct {
	events     chan eventdriven.EventRef
	reconnects chan struct{}
}

func newRecordingHandler() *recordingHandler {
	return &recordingHandler{events: make(chan eventdriven.EventRef, 10), reconnects: make(chan struct{}, 10)}
}

func (h *recordingHandler) HandleEvent(ref eventdriven.EventRef) {
	h.events <- ref
}

func (h *recordingHandler) HandleReconnect() {
	h.reconnects <- struct{}{}
}

func (h *recordingHandler) nextEvent(t *testing.T) eventdriven.EventRef {
	t.Helper()
	select {
	case ref := <-h.events:
		return ref
	case <-time.After(receiveTimeout):
		t.Fatal("no event received")
		return eventdriven.EventRef{}
	}
}

// テンプレートから複製した DB で Listener を動かし、LISTEN を張り終えるまで待つ。テストの終わりに止める
func startListener(t *testing.T) (*sql.DB, *recordingHandler, <-chan struct{}) {
	t.Helper()
	db, dsn := rdbtest.NewDatabase(t)
	listener := eventdriven.NewListener(dsn)
	handler := newRecordingHandler()
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		listener.Run(ctx, handler)
		close(stopped)
	}()
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
	select {
	case <-listener.Ready():
	case <-time.After(receiveTimeout):
		t.Fatal("listener not ready")
	}
	return db, handler, stopped
}

func notify(t *testing.T, db *sql.DB, payload string) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), `SELECT pg_notify('domain_events_channel', $1)`, payload)
	require.NoError(t, err)
}

func TestListener_DeliversCommittedDomainEvent(t *testing.T) {
	db, handler, _ := startListener(t)
	var entityID, eventID int64
	require.NoError(t, db.QueryRowContext(t.Context(), `INSERT INTO domain_entities (entity_type_id) VALUES (2000) RETURNING id`).Scan(&entityID))
	require.NoError(t, db.QueryRowContext(t.Context(), `
		INSERT INTO domain_events (entity_type_id, entity_id, aggregate_version, event_type_id, event_type_version, payload, created_at)
		VALUES (2000, $1, 1, 2000, 1, '{}', now()) RETURNING id`, entityID).Scan(&eventID))

	ref := handler.nextEvent(t)

	require.Equal(t, entityID, ref.EntityID)
	require.Equal(t, int64(1), ref.AggregateVersion)
	require.Equal(t, eventID, ref.EventID)
}

func TestListener_SkipsInvalidNotification(t *testing.T) {
	db, handler, _ := startListener(t)

	notify(t, db, "invalid")
	notify(t, db, "2000:2000:1:1:1")

	// 同じ接続の通知は送った順に届くため、次に届くのは読める方
	require.Equal(t, int64(1), handler.nextEvent(t).EntityID)
}

func TestListener_ReportsReconnectAndKeepsListening(t *testing.T) {
	db, handler, _ := startListener(t)

	// Listener の接続を DB の側から切る（DB の再起動・ネットワークの切断の代わり）
	_, err := db.ExecContext(t.Context(), `
		SELECT pg_terminate_backend(pid) FROM pg_stat_activity
		WHERE datname = current_database() AND query LIKE 'LISTEN%' AND pid <> pg_backend_pid()`)
	require.NoError(t, err)

	select {
	case <-handler.reconnects:
	case <-time.After(receiveTimeout):
		t.Fatal("no reconnect reported")
	}
	notify(t, db, "2000:2000:2:1:2")
	require.Equal(t, int64(2), handler.nextEvent(t).EntityID)
}

func TestListener_StopsWhenContextIsCanceled(t *testing.T) {
	_, dsn := rdbtest.NewDatabase(t)
	listener := eventdriven.NewListener(dsn)
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		listener.Run(ctx, newRecordingHandler())
		close(stopped)
	}()
	<-listener.Ready()

	cancel()

	select {
	case <-stopped:
	case <-time.After(receiveTimeout):
		t.Fatal("listener did not stop")
	}
}
