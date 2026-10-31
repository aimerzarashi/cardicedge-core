package eventstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aarondl/sqlboiler/v4/queries/qm"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// Load と EventLoader で、domain_events の読み方と復元を 1 か所にするため分ける
type reader[I domain.ID, E domain.Event[I]] struct {
	registry     *domaindef.Registry
	newID        domain.NewIDFunc[I]
	newEventType domain.NewEventTypeFunc[I, E]
}

type eventStore[I domain.ID, E domain.Event[I], S any] struct {
	reader[I, E]
	initialState domain.InitialStateFunc[S]
	evolve       domain.EvolveFunc[S, E]
}

func New[I domain.ID, E domain.Event[I], S any](
	registry *domaindef.Registry,
	newID domain.NewIDFunc[I],
	newEventType domain.NewEventTypeFunc[I, E],
	initialState domain.InitialStateFunc[S],
	evolve domain.EvolveFunc[S, E],
) domain.EventStore[I, E, S] {
	return &eventStore[I, E, S]{
		reader:       reader[I, E]{registry: registry, newID: newID, newEventType: newEventType},
		initialState: initialState,
		evolve:       evolve,
	}
}

// 同じ集約・同じ版のイベントの衝突（同時の更新）を、一意制約違反のエラーではなく、行が返らないことで知る。
// エラーにすると PostgreSQL が ERROR のログを出し（衝突は異常ではないのにログが埋まる）、トランザクションも中断されるため。
// sqlboiler の Insert は違反をエラーで返し、Upsert（DO NOTHING）は行が返らないと nil を返して衝突に気づけないため、SQL を直接書く。
// 版が続いていること（版 1 か、直前の版が既にある）も条件にし、どこから呼ばれても版が飛ばないことをここで守る
// （各集約の Decide が「今の版 + 1」で作るのに任せず、保存する場所で保証する。core#24）
const saveEventQuery = `
	INSERT INTO domain_events (entity_type_id, entity_id, aggregate_version, event_type_id, event_type_version, payload, created_at)
	SELECT $1::smallint, $2::bigint, $3::bigint, $4::smallint, $5::int, $6::jsonb, $7::timestamptz
	WHERE $3::bigint = 1 OR EXISTS (
	  SELECT 1 FROM domain_events WHERE entity_type_id = $1::smallint AND entity_id = $2::bigint AND aggregate_version = $3::bigint - 1
	)
	ON CONFLICT (entity_type_id, entity_id, aggregate_version) DO NOTHING
	RETURNING id
`

// 行が返らなかったときに、衝突か版の飛びかを見分ける
const versionExistsQuery = `
	SELECT EXISTS (SELECT 1 FROM domain_events WHERE entity_type_id = $1 AND entity_id = $2 AND aggregate_version = $3)
`

// イベントを保存したログ（1 イベント 1 行）。項目の名前は eventdriven の処理のログとそろえ、event_id で保存と処理をつなぐ。
// 確定のあとに出す（取り消された保存を記録に残さないため）。InfoContext にするのは、OpenTelemetry の文脈を ctx に載せたとき
// トレース ID が付くようにするため
func logSaved(ctx context.Context, eventID int64, entityType domain.EntityType, entityID, version int64, eventType domain.EventName, occurredAt time.Time) {
	rdb.AfterCommit(ctx, func(ctx context.Context) {
		slog.InfoContext(ctx, "eventstore: event saved",
			"event_id", eventID, "entity_type", entityType.String(), "entity_id", entityID,
			"aggregate_version", version, "event_type", eventType.String(), "occurred_at", occurredAt)
	})
}

func (s *eventStore[I, E, S]) Save(ctx context.Context, events []E) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	var zeroID I
	entityTypeID, err := s.registry.EntityTypeID(zeroID.EntityType())
	if err != nil {
		return err
	}
	for _, event := range events {
		meta := event.GetMeta()
		eventTypeID, err := s.registry.EventTypeID(entityTypeID, event.EventType())
		if err != nil {
			return err
		}
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		var id int64
		err = exec.QueryRowContext(ctx, saveEventQuery,
			entityTypeID.Int16(), meta.EntityID.Int64(), meta.AggregateVersion, eventTypeID.Int16(), event.SchemaVersion(), payload,
			// DB の現在時刻ではなく、イベントの発生時刻を残す（設計メモ 6 節の 4）
			meta.OccurredAt,
		).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			var exists bool
			if err := exec.QueryRowContext(ctx, versionExistsQuery, entityTypeID.Int16(), meta.EntityID.Int64(), meta.AggregateVersion).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("%w: version %d", domain.ErrVersionNotConsecutive, meta.AggregateVersion)
			}
			return domain.ErrVersionConflict
		}
		if err != nil {
			return err
		}
		logSaved(ctx, id, zeroID.EntityType(), meta.EntityID.Int64(), meta.AggregateVersion, event.EventType(), meta.OccurredAt)
	}
	return nil
}

func (s *eventStore[I, E, S]) Load(ctx context.Context, entityID I) (S, error) {
	var zero S
	events, err := s.loadEvents(ctx, entityID)
	if err != nil {
		return zero, err
	}
	state := s.initialState()
	for _, loaded := range events {
		if state, err = s.evolve(state, loaded.event); err != nil {
			return zero, err
		}
	}
	return state, nil
}

// 復元したイベントと、元の行（処理のログが event_id 等を引くため）
type decodedEvent[E any] struct {
	event E
	row   *models.DomainEvent
}

func (r *reader[I, E]) loadEvents(ctx context.Context, entityID I) ([]decodedEvent[E], error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	entityTypeID, err := r.registry.EntityTypeID(entityID.EntityType())
	if err != nil {
		return nil, err
	}
	// entity_id は種別をまたいで重なりうる（集約ごとに別の ID 型）ため、entity_type_id でも絞る（設計メモ 6 節の 3）
	rows, err := models.DomainEvents(
		models.DomainEventWhere.EntityTypeID.EQ(entityTypeID.Int16()),
		models.DomainEventWhere.EntityID.EQ(entityID.Int64()),
		qm.OrderBy(models.DomainEventColumns.AggregateVersion+" ASC"),
	).All(ctx, exec)
	if err != nil {
		return nil, err
	}
	events := make([]decodedEvent[E], 0, len(rows))
	for _, row := range rows {
		event, err := r.decode(row)
		if err != nil {
			return nil, err
		}
		events = append(events, decodedEvent[E]{event: event, row: row})
	}
	return events, nil
}

func (r *reader[I, E]) decode(row *models.DomainEvent) (E, error) {
	var zero E
	id, err := r.newID(row.EntityID)
	if err != nil {
		return zero, err
	}
	name, err := r.registry.EventTypeName(domaindef.NewEventTypeID(row.EventTypeID))
	if err != nil {
		return zero, err
	}
	meta := domain.EventMeta[I]{EntityID: id, AggregateVersion: row.AggregateVersion, OccurredAt: row.CreatedAt}
	return r.newEventType(name).Unmarshal(meta, row.EventTypeVersion, row.Payload)
}
