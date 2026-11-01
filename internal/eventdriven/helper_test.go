package eventdriven_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/logtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

// テストの中だけの購読者の名前
const testProjectionName = "eventdriven_test"

// テストの中だけの購読者。複製した DB（購読者の行は入っていない）で動かすため、id は本物の購読者と重なってもよい
var testSubscribers = []eventdriven.SubscriberDef{eventdriven.ProjectionSubscriber(1, testProjectionName)}

// このパッケージのテストは、テンプレートから複製した自分専用の DB で動かす（Hub・Listener は commit を見るため、
// ほかのテストもそろえて、パッケージの中の DB の使い方を 1 つにする）
func isolatedDatabase(t *testing.T) context.Context {
	t.Helper()
	db, _ := rdbtest.NewDatabase(t)
	return rdb.WithDB(t.Context(), db)
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
	return eventdriven.Options{WorkerCount: workerCount, PollInterval: time.Hour, RetryInterval: time.Minute, MaxFailures: 10}
}

// 通知は非同期に届くため、届くのを待つ上限。届かないことの確認には使わない（待つだけ遅くなるため）
const receiveTimeout = 5 * time.Second

// 投影が書いた状態を覚えるだけの、テストの中だけの投影（operator を購読する）
type writes struct {
	upserted []countingReadModel
	deleted  []countingReadModel
}

func (w *writes) spec() eventdriven.ProjectionSpec[countingReadModel] {
	return eventdriven.ProjectionSpec[countingReadModel]{
		Name:          testProjectionName,
		Subscriptions: []domain.EntityType{operator.Types().EntityType},
		Upsert: func(_ context.Context, state countingReadModel) error {
			w.upserted = append(w.upserted, state)
			return nil
		},
		Delete: func(_ context.Context, state countingReadModel) error {
			w.deleted = append(w.deleted, state)
			return nil
		},
	}
}

// テストの中だけの購読者と、operator のイベントの読み方を渡した Hub を作る（投影は登録しない）
func newHub(t *testing.T) (context.Context, *eventdriven.Hub) {
	t.Helper()
	ctx := isolatedDatabase(t)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	hub, err := eventdriven.NewHub(testSubscribers, registry, testOptions(1), eventstore.NewEventLoader(registry, operator.NewID, operator.NewEventType))
	require.NoError(t, err)
	return ctx, hub
}

// 処理のログのうち、指した集約・購読者のものが want 件出るのを待って、出た順に返す
func waitProcessedLogs(t *testing.T, records *logtest.Records, subscriberName string, entityID int64, want int) []logtest.Record {
	t.Helper()
	var matched []logtest.Record
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		matched = nil
		for _, row := range records.Find("eventdriven: event processed") {
			if row.Attrs["subscriber_name"] == subscriberName && row.Attrs["entity_id"] == entityID {
				matched = append(matched, row)
			}
		}
		assert.Len(c, matched, want)
	}, receiveTimeout, 10*time.Millisecond)
	return matched
}

// 処理のスパンを記録するプロバイダーを、options に載せて返す
func withSpanRecorder(options eventdriven.Options) (eventdriven.Options, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	options.TracerProvider = sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	return options, recorder
}

// 指した名前のスパンが want 個終わるのを待って返す
func waitSpans(t *testing.T, recorder *tracetest.SpanRecorder, name string, want int) []sdktrace.ReadOnlySpan {
	t.Helper()
	var matched []sdktrace.ReadOnlySpan
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		matched = nil
		for _, span := range recorder.Ended() {
			if span.Name() == name {
				matched = append(matched, span)
			}
		}
		assert.Len(c, matched, want)
	}, receiveTimeout, 10*time.Millisecond)
	return matched
}

func spanAttributes(span sdktrace.ReadOnlySpan) map[string]any {
	found := map[string]any{}
	for _, kv := range span.Attributes() {
		found[string(kv.Key)] = kv.Value.AsInterface()
	}
	return found
}

// 保存した traceparent から読み戻した SpanContext は remote が true になるため、trace id と span id で比べる
func assertSameSpan(t *testing.T, want, got trace.SpanContext) {
	t.Helper()
	assert.Equal(t, want.TraceID(), got.TraceID())
	assert.Equal(t, want.SpanID(), got.SpanID())
}
