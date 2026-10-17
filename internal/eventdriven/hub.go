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
}

type Hub struct {
	subscribers *Subscribers
	registry    *domaindef.Registry
	options     Options
	loaders     map[domaindef.EntityTypeID]eventstore.EventLoader
	projections map[int16]*registeredProjection
	// 集約の種別 → その種別を購読する投影の id
	routes    map[domaindef.EntityTypeID][]int16
	scheduler *scheduler
	// 再接続の合図。容量 1 で、待たずに送る（探している最中に何回来ても、終わった後に 1 回探すだけ）
	pollNow chan struct{}
	started atomic.Bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

type registeredProjection struct {
	name          string
	entityTypeIDs []domaindef.EntityTypeID
	projection    projection
}

// loaders は購読されうる集約ごとのイベントの読み方。購読する種別の読み方が無ければ、登録のときにエラーにする
func NewHub(subscribers *Subscribers, registry *domaindef.Registry, options Options, loaders ...eventstore.EventLoader) (*Hub, error) {
	if options.WorkerCount < 1 {
		return nil, fmt.Errorf("eventdriven: WorkerCount must be at least 1: %d", options.WorkerCount)
	}
	if options.PollInterval <= 0 {
		return nil, fmt.Errorf("eventdriven: PollInterval must be positive: %s", options.PollInterval)
	}
	hub := &Hub{
		subscribers: subscribers,
		registry:    registry,
		options:     options,
		loaders:     make(map[domaindef.EntityTypeID]eventstore.EventLoader, len(loaders)),
		projections: make(map[int16]*registeredProjection),
		routes:      make(map[domaindef.EntityTypeID][]int16),
		scheduler:   newScheduler(),
		pollNow:     make(chan struct{}, 1),
	}
	var errs []error
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

func (h *Hub) register(kind, name string, subscriptions []domain.EntityType, p projection) error {
	// 動き出した後に足すと、ワーカーが routes を読むのと書き換えが重なるため
	if h.started.Load() {
		return fmt.Errorf("eventdriven: projection %q is registered after Start", name)
	}
	id, err := h.subscribers.id(kind, name)
	if err != nil {
		return err
	}
	if _, ok := h.projections[id]; ok {
		return fmt.Errorf("eventdriven: projection %q is already registered", name)
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
		return fmt.Errorf("eventdriven: projection %q: %w", name, err)
	}
	if err := h.subscribers.verifySubscriptions(id, entityTypeIDs); err != nil {
		return fmt.Errorf("eventdriven: projection %q: %w", name, err)
	}
	h.projections[id] = &registeredProjection{name: name, entityTypeIDs: entityTypeIDs, projection: p}
	for _, entityTypeID := range entityTypeIDs {
		h.routes[entityTypeID] = append(h.routes[entityTypeID], id)
	}
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
}

// ListenerHandler。Listener から呼ばれる
func (h *Hub) HandleEvent(ref EventRef) {
	for _, subscriberID := range h.routes[ref.EntityTypeID] {
		h.scheduler.enqueue(projectionKey{subscriberID: subscriberID, entityTypeID: ref.EntityTypeID, entityID: ref.EntityID})
	}
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
// 失敗はログに出すだけで、次の知らせか Poller で作り直す（失敗し続けるときの扱いは core#18）
func (h *Hub) process(ctx context.Context, key projectionKey) {
	registered := h.projections[key.subscriberID]
	defer h.scheduler.done(key)
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "eventdriven: projection panicked",
				"projection", registered.name, "entity_type_id", key.entityTypeID.Int16(), "entity_id", key.entityID,
				"panic", r, "stack", string(debug.Stack()))
		}
	}()
	if err := h.project(ctx, key, registered.projection); err != nil && ctx.Err() == nil {
		slog.ErrorContext(ctx, "eventdriven: projection failed",
			"projection", registered.name, "entity_type_id", key.entityTypeID.Int16(), "entity_id", key.entityID, "error", err)
	}
}

func (h *Hub) project(ctx context.Context, key projectionKey, p projection) error {
	events, err := h.loaders[key.entityTypeID].LoadEvents(ctx, key.entityID)
	if err != nil || len(events) == 0 {
		return err
	}
	version := events[len(events)-1].AggregateVersion
	_, err = rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		advanced, err := advanceCheckpoint(ctx, key, version)
		if err != nil {
			return struct{}{}, err
		}
		if advanced {
			if err := p.project(ctx, events); err != nil {
				return struct{}{}, err
			}
		}
		// 作り直さなかった（同じか新しい版で作ってある）ときも消す。残すと Poller が拾い続けるため
		return struct{}{}, removePending(ctx, key, version)
	})
	return err
}
