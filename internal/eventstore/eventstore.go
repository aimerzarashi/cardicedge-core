package eventstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/aarondl/sqlboiler/v4/queries/qm"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

type eventStore[I domain.ID, E domain.Event[I], S any] struct {
	registry     *domaindef.Registry
	newID        domain.NewIDFunc[I]
	newEventType domain.NewEventTypeFunc[I, E]
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
	return &eventStore[I, E, S]{registry: registry, newID: newID, newEventType: newEventType, initialState: initialState, evolve: evolve}
}

// 同じ集約・同じ版のイベントの衝突（同時の更新）を、一意制約違反のエラーではなく、行が返らないことで知る。
// エラーにすると PostgreSQL が ERROR のログを出し（衝突は異常ではないのにログが埋まる）、トランザクションも中断されるため。
// sqlboiler の Insert は違反をエラーで返し、Upsert（DO NOTHING）は行が返らないと nil を返して衝突に気づけないため、SQL を直接書く
const saveEventQuery = `
	INSERT INTO domain_events (entity_type_id, entity_id, aggregate_version, event_type_id, event_type_version, payload, created_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (entity_type_id, entity_id, aggregate_version) DO NOTHING
	RETURNING id
`

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
			return domain.ErrVersionConflict
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *eventStore[I, E, S]) Load(ctx context.Context, entityID I) (S, error) {
	var zero S
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return zero, err
	}
	entityTypeID, err := s.registry.EntityTypeID(entityID.EntityType())
	if err != nil {
		return zero, err
	}
	// entity_id は種別をまたいで重なりうる（集約ごとに別の ID 型）ため、entity_type_id でも絞る（設計メモ 6 節の 3）
	rows, err := models.DomainEvents(
		models.DomainEventWhere.EntityTypeID.EQ(entityTypeID.Int16()),
		models.DomainEventWhere.EntityID.EQ(entityID.Int64()),
		qm.OrderBy(models.DomainEventColumns.AggregateVersion+" ASC"),
	).All(ctx, exec)
	if err != nil {
		return zero, err
	}

	state := s.initialState()
	for _, row := range rows {
		event, err := s.decode(row)
		if err != nil {
			return zero, err
		}
		if state, err = s.evolve(state, event); err != nil {
			return zero, err
		}
	}
	return state, nil
}

func (s *eventStore[I, E, S]) decode(row *models.DomainEvent) (E, error) {
	var zero E
	id, err := s.newID(row.EntityID)
	if err != nil {
		return zero, err
	}
	name, err := s.registry.EventTypeName(domaindef.NewEventTypeID(row.EventTypeID))
	if err != nil {
		return zero, err
	}
	meta := domain.EventMeta[I]{EntityID: id, AggregateVersion: row.AggregateVersion, OccurredAt: row.CreatedAt}
	return s.newEventType(name).Unmarshal(meta, row.EventTypeVersion, row.Payload)
}
