package typeregistry

import (
	"context"
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// Registry は domain_entity_types・domain_event_types の name と id の対応。
// パッケージの変数にせず、Load の結果を identifier・eventstore に渡すのは、読み込み忘れをコンパイル時にわかるようにし、
// 依存を引数に表すため（参照の api はパッケージの変数だった）。起動後は読み取り専用のため錠を持たない
type Registry struct {
	entityTypeIDs  map[string]int16
	eventTypeIDs   map[int16]map[string]int16 // イベント種別名は集約の中でだけ一意なため、集約ごとに分ける
	eventTypeNames map[int16]string
}

func Load(ctx context.Context) (*Registry, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	entityTypes, err := models.DomainEntityTypes().All(ctx, exec)
	if err != nil {
		return nil, fmt.Errorf("failed to load domain_entity_types: %w", err)
	}
	eventTypes, err := models.DomainEventTypes().All(ctx, exec)
	if err != nil {
		return nil, fmt.Errorf("failed to load domain_event_types: %w", err)
	}

	r := &Registry{entityTypeIDs: map[string]int16{}, eventTypeIDs: map[int16]map[string]int16{}, eventTypeNames: map[int16]string{}}
	for _, row := range entityTypes {
		r.entityTypeIDs[row.Name] = row.ID
	}
	for _, row := range eventTypes {
		if r.eventTypeIDs[row.EntityTypeID] == nil {
			r.eventTypeIDs[row.EntityTypeID] = map[string]int16{}
		}
		r.eventTypeIDs[row.EntityTypeID][row.Name] = row.ID
		r.eventTypeNames[row.ID] = row.Name
	}
	return r, nil
}

func (r *Registry) EntityTypeID(name string) (int16, error) {
	id, ok := r.entityTypeIDs[name]
	if !ok {
		return 0, fmt.Errorf("entity type not found: %s", name)
	}
	return id, nil
}

func (r *Registry) EventTypeID(entityTypeID int16, name string) (int16, error) {
	id, ok := r.eventTypeIDs[entityTypeID][name]
	if !ok {
		return 0, fmt.Errorf("event type not found: entity_type_id=%d, name=%s", entityTypeID, name)
	}
	return id, nil
}

func (r *Registry) EventTypeName(id int16) (string, error) {
	name, ok := r.eventTypeNames[id]
	if !ok {
		return "", fmt.Errorf("event type id not found: %d", id)
	}
	return name, nil
}

// アプリケーションの起動時に呼ぶ
func (r *Registry) Verify(aggregates ...domain.AggregateTypes) error {
	var errs []error
	for _, aggregate := range aggregates {
		entityTypeID, err := r.EntityTypeID(aggregate.EntityType)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, name := range aggregate.EventTypes {
			if _, err := r.EventTypeID(entityTypeID, name); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
