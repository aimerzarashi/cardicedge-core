package eventdriven

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

var ErrListenerStopped = errors.New("eventdriven: listener stopped before ready")

type Options struct {
	WorkerCount int
	// やることの表を読む間隔。起動時と再接続時は間隔によらずすぐ読むため、間隔で拾うのは失敗した集約の試し直しと、
	// 投影を足したとき・作り直すときに入れた行
	PollInterval time.Duration
	// ポリシーが失敗してから、やり直すまでの間隔。投影には使わない（投影は何度作り直しても害が無く、失敗の記録を持たない）
	RetryInterval time.Duration
	// ポリシーが 1 つの集約で失敗できる回数。この回数に達すると、デッドレターとしてやり直さなくなる
	MaxFailures int
	// 処理のスパンを記録するプロバイダー。nil ならグローバル。gqltrace と同じく、テストが記録用のものを渡せるようにする
	// （グローバルを差し替えると、並行して動くほかのテストに影響するため）
	TracerProvider trace.TracerProvider
}

const tracerName = "gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"

type Hub struct {
	// この Hub が動くサービスの購読者（コードの一覧から渡す）と、その (kind, name) から id を引く表
	subscriberDefs []SubscriberDef
	subscriberIDs  map[subscriberKey]int16
	registry       *domaindef.Registry
	options        Options
	tracer         trace.Tracer
	loaders        map[domaindef.EntityTypeID]eventstore.EventLoader
	registered     map[int16]*registeredSubscriber
	// 集約の種別 → その種別を購読する購読者（投影・ポリシー）の id
	routes map[domaindef.EntityTypeID][]int16
	// 集約の種別 → 通知を見るだけの口（Watch）
	watchers  map[domaindef.EntityTypeID][]func(EventRef)
	scheduler *scheduler
	// 複数台で、1 つの単位を 1 台だけが処理するためのロック（lock.go）
	locks unitLocks
	// 再接続の合図。容量 1 で、待たずに送る（探している最中に何回来ても、終わった後に 1 回探すだけ）
	pollNow chan struct{}
	started atomic.Bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// 投影かポリシーのどちらか一方だけを持つ（kind で分ける）
type registeredSubscriber struct {
	kind          string
	name          string
	entityTypeIDs []domaindef.EntityTypeID
	projection    projection
	policy        PolicyHandleFunc
}

// subscribers は、この Hub が動くサービスの購読者（id はコードの一覧で決める。core#33）。
// loaders は購読されうる集約ごとのイベントの読み方。購読する種別の読み方が無ければ、登録のときにエラーにする
func NewHub(subscribers []SubscriberDef, registry *domaindef.Registry, options Options, loaders ...eventstore.EventLoader) (*Hub, error) {
	if options.WorkerCount < 1 {
		return nil, fmt.Errorf("eventdriven: WorkerCount must be at least 1: %d", options.WorkerCount)
	}
	if options.PollInterval <= 0 {
		return nil, fmt.Errorf("eventdriven: PollInterval must be positive: %s", options.PollInterval)
	}
	if options.RetryInterval <= 0 {
		return nil, fmt.Errorf("eventdriven: RetryInterval must be positive: %s", options.RetryInterval)
	}
	if options.MaxFailures < 1 {
		return nil, fmt.Errorf("eventdriven: MaxFailures must be at least 1: %d", options.MaxFailures)
	}
	provider := options.TracerProvider
	if provider == nil {
		provider = otel.GetTracerProvider()
	}
	hub := &Hub{
		tracer:         provider.Tracer(tracerName),
		subscriberDefs: subscribers,
		subscriberIDs:  make(map[subscriberKey]int16, len(subscribers)),
		registry:       registry,
		options:        options,
		loaders:        make(map[domaindef.EntityTypeID]eventstore.EventLoader, len(loaders)),
		registered:     make(map[int16]*registeredSubscriber),
		routes:         make(map[domaindef.EntityTypeID][]int16),
		watchers:       make(map[domaindef.EntityTypeID][]func(EventRef)),
		scheduler:      newScheduler(),
		pollNow:        make(chan struct{}, 1),
	}
	var errs []error
	ids := make(map[int16]bool, len(subscribers))
	for _, def := range subscribers {
		key := subscriberKey{kind: def.Kind, name: def.Name}
		if _, ok := hub.subscriberIDs[key]; ok || ids[def.ID] {
			errs = append(errs, fmt.Errorf("duplicate subscriber: id=%d, kind=%s, name=%s", def.ID, def.Kind, def.Name))
			continue
		}
		hub.subscriberIDs[key] = def.ID
		ids[def.ID] = true
	}
	for _, loader := range loaders {
		entityTypeID, err := registry.EntityTypeID(loader.EntityType())
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if _, ok := hub.loaders[entityTypeID]; ok {
			errs = append(errs, fmt.Errorf("duplicate event loader: %s", loader.EntityType()))
			continue
		}
		hub.loaders[entityTypeID] = loader
	}
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("eventdriven: %w", err)
	}
	return hub, nil
}

func (h *Hub) register(sub *registeredSubscriber, subscriptions []domain.EntityType) error {
	kind, name := sub.kind, sub.name
	// 動き出した後に足すと、ワーカーが routes を読むのと書き換えが重なるため
	if h.started.Load() {
		return fmt.Errorf("eventdriven: %s %q is registered after Start", kind, name)
	}
	id, ok := h.subscriberIDs[subscriberKey{kind: kind, name: name}]
	if !ok {
		return fmt.Errorf("%w: kind=%s, name=%s", ErrSubscriberNotFound, kind, name)
	}
	if _, ok := h.registered[id]; ok {
		return fmt.Errorf("eventdriven: %s %q is already registered", kind, name)
	}
	var errs []error
	entityTypeIDs := make([]domaindef.EntityTypeID, 0, len(subscriptions))
	seen := make(map[domaindef.EntityTypeID]bool, len(subscriptions))
	for _, entityType := range subscriptions {
		entityTypeID, err := h.registry.EntityTypeID(entityType)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if seen[entityTypeID] {
			errs = append(errs, fmt.Errorf("duplicate subscription: %s", entityType))
			continue
		}
		seen[entityTypeID] = true
		if _, ok := h.loaders[entityTypeID]; !ok {
			errs = append(errs, fmt.Errorf("no event loader for subscription: %s", entityType))
			continue
		}
		entityTypeIDs = append(entityTypeIDs, entityTypeID)
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("eventdriven: %s %q: %w", kind, name, err)
	}
	sub.entityTypeIDs = entityTypeIDs
	h.registered[id] = sub
	for _, entityTypeID := range entityTypeIDs {
		h.routes[entityTypeID] = append(h.routes[entityTypeID], id)
	}
	return nil
}

// 集約の種別の通知（EventRef）を見るだけの口を足す。投影・ポリシーと違い、やることの表・チェックポイント・失敗の記録を持たず、
// 知らせが届いた台で呼ぶだけ（取りこぼしは見る側が自分で読み直して埋める）。watch は Listener の受信を止めないよう、待たずに返すこと
// （Listener は 1 本で全種別の通知を受けるため、待つと投影・ポリシーへの知らせも止まる）。Start の前に足す
func (h *Hub) Watch(entityType domain.EntityType, watch func(EventRef)) error {
	if h.started.Load() {
		return fmt.Errorf("eventdriven: watcher of %s is registered after Start", entityType)
	}
	entityTypeID, err := h.registry.EntityTypeID(entityType)
	if err != nil {
		return fmt.Errorf("eventdriven: watcher: %w", err)
	}
	h.watchers[entityTypeID] = append(h.watchers[entityTypeID], watch)
	return nil
}

// ctx には rdb.WithDB で DB を載せる（ワーカーがトランザクションを始めるのに使う）。LISTEN を張り終えてから返す
// （張る前に commit されたイベントの知らせは届かないため）。投影はすべて Start の前に登録する
func (h *Hub) Start(ctx context.Context, listener *Listener) error {
	if !h.started.CompareAndSwap(false, true) {
		return errors.New("eventdriven: hub is already started")
	}
	ctx, h.cancel = context.WithCancel(ctx)

	stopped := make(chan struct{})
	h.wg.Go(func() {
		defer close(stopped)
		listener.Run(ctx, h)
	})
	select {
	case <-listener.Ready():
	case <-stopped:
		h.Stop()
		return ErrListenerStopped
	case <-ctx.Done():
		h.Stop()
		return ctx.Err()
	}

	for range h.options.WorkerCount {
		h.wg.Go(func() {
			h.work(ctx)
		})
	}
	h.wg.Go(func() {
		h.poll(ctx)
	})
	return nil
}

// 処理中のトランザクションは ctx の取り消しで中断する（作り直せなかった分はチェックポイントが進まないため、
// 次に起動したときに Poller が拾う。最後まで待つと、cmd/app のシャットダウンの時間に収める工夫が要る）
func (h *Hub) Stop() {
	if h.cancel == nil {
		return
	}
	h.cancel()
	h.scheduler.close()
	h.wg.Wait()
	h.locks.close()
}

// ListenerHandler。Listener から呼ばれる
func (h *Hub) HandleEvent(ref EventRef) {
	for _, watch := range h.watchers[ref.EntityTypeID] {
		callWatcher(watch, ref)
	}
	for _, subscriberID := range h.routes[ref.EntityTypeID] {
		h.scheduler.enqueue(workKey{subscriberID: subscriberID, entityTypeID: ref.EntityTypeID, entityID: ref.EntityID})
	}
}

// 見る側の誤りで Listener の受信ループを落とさない（落ちると全種別の通知が止まる）
func callWatcher(watch func(EventRef), ref EventRef) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("eventdriven: watcher panicked", "entity_type_id", ref.EntityTypeID, "entity_id", ref.EntityID, "panic", r, "stack", string(debug.Stack()))
		}
	}()
	watch(ref)
}

// ListenerHandler。つながり直す間の知らせは失われているため、間隔を待たずにやることの表を読む
func (h *Hub) HandleReconnect() {
	select {
	case h.pollNow <- struct{}{}:
	default:
	}
}

func (h *Hub) work(ctx context.Context) {
	for {
		key, ok := h.scheduler.next()
		if !ok {
			return
		}
		h.process(ctx, key)
	}
}

// panic も拾い、エラーと同じ扱いにする（拾わないとプロセスごと落ち、キーの印も戻らない）。
// 投影の失敗はログに出すだけで、次の知らせか Poller で作り直す。ポリシーの失敗は失敗の記録に残し、間隔と上限で扱う（policy.go）
func (h *Hub) process(ctx context.Context, key workKey) {
	registered := h.registered[key.subscriberID]
	lock, acquired, err := h.locks.tryLock(ctx, key)
	if err != nil || !acquired {
		if err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "eventdriven: locking unit failed",
				"kind", registered.kind, "name", registered.name, "entity_type_id", key.entityTypeID.Int16(), "entity_id", key.entityID, "error", err)
		}
		h.scheduler.skip(key)
		return
	}
	defer h.scheduler.done(key)
	// 取り消された ctx でも外す（外せないと接続を捨てることになり、ほかの単位のロックまで外れるため）
	defer func() {
		if err := h.locks.unlock(context.WithoutCancel(ctx), lock); err != nil {
			slog.ErrorContext(ctx, "eventdriven: unlocking unit failed",
				"kind", registered.kind, "name", registered.name, "entity_type_id", key.entityTypeID.Int16(), "entity_id", key.entityID, "error", err)
		}
	}()
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "eventdriven: subscriber panicked",
				"kind", registered.kind, "name", registered.name, "entity_type_id", key.entityTypeID.Int16(), "entity_id", key.entityID,
				"panic", r, "stack", string(debug.Stack()))
		}
	}()
	if registered.kind == KindPolicy {
		err = h.runPolicy(ctx, key, registered)
	} else {
		err = h.project(ctx, key, registered)
	}
	if err != nil && ctx.Err() == nil {
		slog.ErrorContext(ctx, "eventdriven: subscriber failed",
			"kind", registered.kind, "name", registered.name, "entity_type_id", key.entityTypeID.Int16(), "entity_id", key.entityID, "error", err)
	}
}

func (h *Hub) project(ctx context.Context, key workKey, registered *registeredSubscriber) error {
	events, err := h.loaders[key.entityTypeID].LoadEvents(ctx, key.entityID)
	if err != nil || len(events) == 0 {
		return err
	}
	version := events[len(events)-1].AggregateVersion
	start := time.Now()
	// 投影は集約のイベントをすべて当てて作り直すので、処理のログは前の版より後の（新しく反映した）イベントだけに出す
	var previous int64
	var span trace.Span
	// panic でも閉じ忘れない（End は 2 回目以降を無視する）
	defer func() {
		if span != nil {
			span.End()
		}
	}()
	_, err = rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		var err error
		if previous, err = readCheckpoint(ctx, key); err != nil {
			return struct{}{}, err
		}
		// どのイベントが新しいかはチェックポイントを読むまでわからないため、読み込みの前には始められない。
		// Transact は中の関数を試し直さない（1 回だけ呼ぶ）ので、スパンは 1 つだけ始まる
		ctx, span = h.startProjectionSpan(ctx, key, registered, events, previous)
		advanced, err := advanceCheckpoint(ctx, key, version)
		if err != nil {
			return struct{}{}, err
		}
		if advanced {
			if err := registered.projection.project(ctx, events); err != nil {
				return struct{}{}, err
			}
		}
		// 作り直さなかった（同じか新しい版で作ってある）ときも消す。残すと Poller が拾い続けるため
		return struct{}{}, removePending(ctx, key, version)
	})
	// 確定（commit）の結果までスパンに含める
	if span != nil {
		if err != nil && ctx.Err() == nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}
	if ctx.Err() != nil {
		return err
	}
	// 失敗はトランザクション全体が戻るので、新しいイベントすべてが失敗。かかった時間は単位全体のもの
	for _, event := range events {
		if event.AggregateVersion > previous {
			h.logProcessed(ctx, key, registered, event, start, err)
		}
	}
	return err
}

// イベントを処理したログ（購読者ごと・イベントごとに 1 行）。保存のログ（eventstore）と項目の名前をそろえ、event_id でつなぐ。
// lag_ms（発生から処理が終わるまで）で、投影の遅れを判断する。失敗の理由は、ここに error を重ねず今の ERROR・WARN のログに任せる
// （二重に出さないため。この行の result で、どのイベントが失敗したかだけを残す）。取り消された ctx（停止中）では出さない
func (h *Hub) logProcessed(ctx context.Context, key workKey, registered *registeredSubscriber, event eventstore.LoadedEvent, start time.Time, cause error) {
	result := "success"
	if cause != nil {
		result = "failure"
	}
	now := time.Now()
	// 処理のスパンの ctx ではなく、保存したときのトレースを載せ、trace_id（と保存のスパンの span_id）をログに付ける。
	// 投影のスパンは別のトレースのため、その trace_id にすると、Loki で元の trace id 1 つで引けなくなる。
	// 無効な SpanContext は何も変えないので、トレースが無いイベントは今のまま
	if event.SpanContext.IsValid() {
		ctx = trace.ContextWithRemoteSpanContext(ctx, event.SpanContext)
	}
	slog.InfoContext(ctx, "eventdriven: event processed",
		"subscriber_kind", registered.kind, "subscriber_name", registered.name,
		"event_id", event.EventID, "entity_type", h.loaders[key.entityTypeID].EntityType().String(), "entity_id", key.entityID,
		"aggregate_version", event.AggregateVersion, "event_type", event.EventType.String(),
		"result", result, "duration_ms", now.Sub(start).Milliseconds(), "lag_ms", now.Sub(event.OccurredAt).Milliseconds())
}

func (h *Hub) subscriberAttributes(key workKey, registered *registeredSubscriber) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("subscriber_kind", registered.kind),
		attribute.String("subscriber_name", registered.name),
		attribute.String("entity_type", h.loaders[key.entityTypeID].EntityType().String()),
		attribute.Int64("entity_id", key.entityID),
	}
}

// 新しく反映するイベントに元のトレースがあれば、新しいトレースのスパンをそのトレースすべてへのリンクつきで始める。
// 件数で親子とリンクを使い分けないのは、どれが新しいイベントかがトランザクションの中でしかわからず、見え方が件数で変わるため。
// 元のトレースが無いイベントだけなら、スパンを作らず nil を返す（seed-master 等で Tempo の一覧が埋まるため）
func (h *Hub) startProjectionSpan(ctx context.Context, key workKey, registered *registeredSubscriber, events []eventstore.LoadedEvent, previous int64) (context.Context, trace.Span) {
	var links []trace.Link
	var eventIDs []int64
	for _, event := range events {
		if event.AggregateVersion <= previous {
			continue
		}
		eventIDs = append(eventIDs, event.EventID)
		if event.SpanContext.IsValid() {
			links = append(links, trace.Link{SpanContext: event.SpanContext})
		}
	}
	if len(links) == 0 {
		return ctx, nil
	}
	attrs := append(h.subscriberAttributes(key, registered),
		attribute.Int64("aggregate_version", events[len(events)-1].AggregateVersion),
		attribute.Int64Slice("event_ids", eventIDs))
	return h.tracer.Start(ctx, "projection "+registered.name,
		trace.WithNewRoot(), trace.WithLinks(links...), trace.WithSpanKind(trace.SpanKindConsumer), trace.WithAttributes(attrs...))
}
