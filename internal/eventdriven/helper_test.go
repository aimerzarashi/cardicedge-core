package eventdriven_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

// テストの中だけの購読者の名前。id は本物の購読者（1000〜）と重ならない値にする（0010 のマイグレーション）
const testProjectionName = "eventdriven_test"

// このパッケージのテストは、テンプレートから複製した自分専用の DB で動かす（Hub・Listener は commit を見るため、
// ほかのテストもそろえて、パッケージの中の DB の使い方を 1 つにする）
func isolatedDatabase(t *testing.T) context.Context {
	t.Helper()
	db, _ := rdbtest.NewDatabase(t)
	return rdb.WithDB(t.Context(), db)
}

// テストの中だけの購読者と、その購読（operator）を足す
func addTestSubscriber(t *testing.T, ctx context.Context, exec rdb.Executor) {
	t.Helper()
	_, err := exec.ExecContext(ctx, `INSERT INTO event_subscribers (id, kind, name) VALUES (1, 'projection', $1)`, testProjectionName)
	require.NoError(t, err)
	_, err = exec.ExecContext(ctx, `INSERT INTO event_subscriptions (subscriber_id, entity_type_id) VALUES (1, 1000)`)
	require.NoError(t, err)
}

// テストの中だけのイベント。集約を削除するイベントがまだ無いため、終端の状態を作るのに使う
type deletedEvent struct{}

// どの集約か、当てたイベントの数、終端かを数えるだけの read model
type countingReadModel struct {
	EntityID int64
	Applied  int
	Deleted  bool
}

func (m countingReadModel) Evolve(event any) countingReadModel {
	switch e := event.(type) {
	case *operator.RegisteredEvent:
		m.EntityID = e.GetMeta().EntityID.Int64()
		m.Applied++
	case *operator.LinkedEvent:
		m.Applied++
	case deletedEvent:
		m.Deleted = true
	}
	return m
}

func (m countingReadModel) IsTerminal() bool {
	return m.Deleted
}

// 間隔で読むことを待たないテストでは、テストの間に間隔が来ないよう長くする
func testOptions(workerCount int) eventdriven.Options {
	return eventdriven.Options{WorkerCount: workerCount, PollInterval: time.Hour}
}

// 通知は非同期に届くため、届くのを待つ上限。届かないことの確認には使わない（待つだけ遅くなるため）
const receiveTimeout = 5 * time.Second
