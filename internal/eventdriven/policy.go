package eventdriven

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// ctx には DB が載る。トランザクションは無く、コマンドが自分で rdb.Transact する
// （コマンドとチェックポイントを同じトランザクションにしないため。2 回目が来てもコマンドの側で無害にする）
type PolicyHandleFunc func(ctx context.Context, event eventstore.LoadedEvent) error

type PolicySpec struct {
	// event_subscribers.name（kind は policy）
	Name string
	// 購読する集約の種別。その種別のイベントをすべて受け取る
	Subscriptions []domain.EntityType
	Handle        PolicyHandleFunc
}

// 宣言・照合に誤りがあれば登録せずにエラーを返す。起動時に呼び、エラーなら起動を止める
func RegisterPolicy(hub *Hub, spec PolicySpec) error {
	if err := spec.validate(); err != nil {
		return fmt.Errorf("eventdriven: policy %q: %w", spec.Name, err)
	}
	return hub.register(&registeredSubscriber{kind: KindPolicy, name: spec.Name, policy: spec.Handle}, spec.Subscriptions)
}

func (s PolicySpec) validate() error {
	var errs []error
	if s.Name == "" {
		errs = append(errs, errors.New("name is required"))
	}
	if len(s.Subscriptions) == 0 {
		errs = append(errs, errors.New("subscriptions is required"))
	}
	if s.Handle == nil {
		errs = append(errs, errors.New("handle is required"))
	}
	return errors.Join(errs...)
}

// 失敗の記録の回数が上限以上なら（デッドレター）、間隔が過ぎていなければ、何もしない。やることの行は残すので、
// 間隔が過ぎたあとに Poller が拾う
func (h *Hub) runPolicy(ctx context.Context, key workKey, registered *registeredSubscriber) error {
	failure, err := readFailure(ctx, key)
	if err != nil {
		return err
	}
	if failure.found && (failure.count >= h.options.MaxFailures || failure.elapsed < h.options.RetryInterval) {
		return nil
	}
	events, err := h.loaders[key.entityTypeID].LoadEvents(ctx, key.entityID)
	if err != nil || len(events) == 0 {
		return err
	}
	lastVersion, err := readCheckpoint(ctx, key)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.AggregateVersion <= lastVersion {
			continue
		}
		start := time.Now()
		// 保存したときのトレースの子にし、ポリシーがコマンドで保存するイベントの traceparent と失敗のログも、このスパンを指す。
		// 投影と違いイベントを 1 つずつ処理するので、親を 1 つに決められる。トレースが無いイベントはスパンにしない
		eventCtx, span := h.startPolicySpan(ctx, key, registered, event)
		if err := handlePolicyEvent(eventCtx, registered.policy, event); err != nil {
			if ctx.Err() != nil {
				endSpan(span)
				return nil
			}
			if span != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			h.logProcessed(ctx, key, registered, event, start, err)
			err = h.recordFailure(eventCtx, key, registered, event, err)
			endSpan(span)
			return err
		}
		endSpan(span)
		h.logProcessed(ctx, key, registered, event, start, nil)
		// 成功のたびに進める。後のイベントで失敗しても、済んだ分をやり直さない
		if _, err := advanceCheckpoint(ctx, key, event.AggregateVersion); err != nil {
			return err
		}
	}
	if failure.found {
		if err := removeFailure(ctx, key); err != nil {
			return err
		}
	}
	return removePending(ctx, key, events[len(events)-1].AggregateVersion)
}

// panic もエラーと同じ扱いにする。イベントごとに拾うのは、失敗したイベントを記録に残して止めるため
func handlePolicyEvent(ctx context.Context, handle PolicyHandleFunc, event eventstore.LoadedEvent) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	return handle(ctx, event)
}

// 後のイベントは処理しない。待てば直る失敗がありうるため、上限に達するまでは ERROR にしない
func (h *Hub) recordFailure(ctx context.Context, key workKey, registered *registeredSubscriber, event eventstore.LoadedEvent, cause error) error {
	count, err := incrementFailure(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to record policy failure (cause: %v): %w", cause, err)
	}
	attrs := []any{
		"policy", registered.name, "entity_type_id", key.entityTypeID.Int16(), "entity_id", key.entityID,
		"aggregate_version", event.AggregateVersion, "failure_count", count, "error", cause,
	}
	if count == h.options.MaxFailures {
		slog.ErrorContext(ctx, "eventdriven: policy became dead letter", attrs...)
		return nil
	}
	slog.WarnContext(ctx, "eventdriven: policy failed", attrs...)
	return nil
}

type failure struct {
	found   bool
	count   int
	elapsed time.Duration
}

// 経過は DB の時刻で数える（書いた時刻も DB の now() で、アプリと DB の時計のずれに左右されないため）
const readFailureQuery = `
	SELECT failure_count, EXTRACT(EPOCH FROM now() - last_failed_at)::float8
	FROM event_subscriber_failures
	WHERE subscriber_id = $1 AND entity_type_id = $2 AND entity_id = $3
`

func readFailure(ctx context.Context, key workKey) (failure, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return failure{}, err
	}
	var count int
	var seconds float64
	err = exec.QueryRowContext(ctx, readFailureQuery, key.subscriberID, key.entityTypeID.Int16(), key.entityID).Scan(&count, &seconds)
	if errors.Is(err, sql.ErrNoRows) {
		return failure{}, nil
	}
	if err != nil {
		return failure{}, err
	}
	return failure{found: true, count: count, elapsed: time.Duration(seconds * float64(time.Second))}, nil
}

const incrementFailureQuery = `
	INSERT INTO event_subscriber_failures (subscriber_id, entity_type_id, entity_id, failure_count, last_failed_at)
	VALUES ($1, $2, $3, 1, now())
	ON CONFLICT (subscriber_id, entity_type_id, entity_id) DO UPDATE
	SET failure_count = event_subscriber_failures.failure_count + 1, last_failed_at = EXCLUDED.last_failed_at
	RETURNING failure_count
`

func incrementFailure(ctx context.Context, key workKey) (int, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var count int
	err = exec.QueryRowContext(ctx, incrementFailureQuery, key.subscriberID, key.entityTypeID.Int16(), key.entityID).Scan(&count)
	return count, err
}

func removeFailure(ctx context.Context, key workKey) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx,
		`DELETE FROM event_subscriber_failures WHERE subscriber_id = $1 AND entity_type_id = $2 AND entity_id = $3`,
		key.subscriberID, key.entityTypeID.Int16(), key.entityID)
	return err
}

// 行が無ければ 0（まだ 1 件も処理していない）
func readCheckpoint(ctx context.Context, key workKey) (int64, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var version int64
	err = exec.QueryRowContext(ctx,
		`SELECT last_version FROM event_subscriber_checkpoints WHERE subscriber_id = $1 AND entity_type_id = $2 AND entity_id = $3`,
		key.subscriberID, key.entityTypeID.Int16(), key.entityID).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return version, err
}

func (h *Hub) startPolicySpan(ctx context.Context, key workKey, registered *registeredSubscriber, event eventstore.LoadedEvent) (context.Context, trace.Span) {
	if !event.SpanContext.IsValid() {
		return ctx, nil
	}
	attrs := append(h.subscriberAttributes(key, registered),
		attribute.Int64("event_id", event.EventID),
		attribute.Int64("aggregate_version", event.AggregateVersion),
		attribute.String("event_type", event.EventType.String()))
	return h.tracer.Start(trace.ContextWithRemoteSpanContext(ctx, event.SpanContext), "policy "+registered.name,
		trace.WithSpanKind(trace.SpanKindConsumer), trace.WithAttributes(attrs...))
}

func endSpan(span trace.Span) {
	if span != nil {
		span.End()
	}
}
