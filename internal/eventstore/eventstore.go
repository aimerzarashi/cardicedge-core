package eventstore

import (
	"context"
	"encoding/json"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/aarondl/sqlboiler/v4/queries/qm"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/typeregistry"
)

type eventStore[I domain.ID, E domain.Event[I], S any] struct {
	registry     *typeregistry.Registry
	newID        domain.NewIDFunc[I]
	newEventType domain.NewEventTypeFunc[I, E]
	initialState domain.InitialStateFunc[S]
	evolve       domain.EvolveFunc[S, E]
}

func New[I domain.ID, E domain.Event[I], S any](
	registry *typeregistry.Registry,
	newID domain.NewIDFunc[I],
	newEventType domain.NewEventTypeFunc[I, E],
	initialState domain.InitialStateFunc[S],
	evolve domain.EvolveFunc[S, E],
) domain.EventStore[I, E, S] {
	return &eventStore[I, E, S]{registry: registry, newID: newID, newEventType: newEventType, initialState: initialState, evolve: evolve}
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
		row := &models.DomainEvent{
			EntityTypeID:     entityTypeID,
			EntityID:         meta.EntityID.Int64(),
			AggregateVersion: meta.AggregateVersion,
			EventTypeID:      eventTypeID,
			EventTypeVersion: event.SchemaVersion(),
			Payload:          payload,
			// DB の現在時刻ではなく、イベントの発生時刻を残す（設計メモ 6 節の 4）
			CreatedAt: meta.OccurredAt,
		}
		if err := row.Insert(ctx, exec, boil.Infer()); err != nil {
			if rdb.IsUniqueViolation(err) {
				return domain.ErrVersionConflict
			}
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
		models.DomainEventWhere.EntityTypeID.EQ(entityTypeID),
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
	name, err := s.registry.EventTypeName(row.EventTypeID)
	if err != nil {
		return zero, err
	}
	meta := domain.EventMeta[I]{EntityID: id, AggregateVersion: row.AggregateVersion, OccurredAt: row.CreatedAt}
	return s.newEventType(name).Unmarshal(meta, row.EventTypeVersion, row.Payload)
}
