package domaindef

import (
	"context"
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// Registry は domain_entity_types・domain_event_types・domain_unique_key_types の name と id の対応。
// パッケージの変数にせず、Load の結果を identifier・eventstore に渡すのは、読み込み忘れをコンパイル時にわかるようにし、
// 依存を引数に表すため（参照の api はパッケージの変数だった）。起動後は読み取り専用のため錠を持たない
type Registry struct {
	entityTypeIDs  map[domain.EntityType]EntityTypeID
	eventTypeIDs   map[EntityTypeID]map[domain.EventName]EventTypeID // イベント種別名は集約の中でだけ一意なため、集約ごとに分ける
	eventTypeNames map[EventTypeID]domain.EventName
	uniqueKeyTypes map[string]bool
}

// domain_entity_types・domain_event_types の id。どちらも DB では SMALLINT だが、型を分けて取り違えをコンパイルで止める
type (
	EntityTypeID int16
	EventTypeID  int16
)

func (id EntityTypeID) Int16() int16 {
	return int16(id)
}

func (id EventTypeID) Int16() int16 {
	return int16(id)
}

// domain_events や通知から読んだ値を型にする（読み込むときだけ使う）
func NewEntityTypeID(value int16) EntityTypeID {
	return EntityTypeID(value)
}

// domain_events や通知から読んだ値を型にする（読み込むときだけ使う）
func NewEventTypeID(value int16) EventTypeID {
	return EventTypeID(value)
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

	uniqueKeyTypes, err := models.DomainUniqueKeyTypes().All(ctx, exec)
	if err != nil {
		return nil, fmt.Errorf("failed to load domain_unique_key_types: %w", err)
	}

	r := &Registry{
		entityTypeIDs:  map[domain.EntityType]EntityTypeID{},
		eventTypeIDs:   map[EntityTypeID]map[domain.EventName]EventTypeID{},
		eventTypeNames: map[EventTypeID]domain.EventName{},
		uniqueKeyTypes: map[string]bool{},
	}
	for _, row := range uniqueKeyTypes {
		r.uniqueKeyTypes[row.Name] = true
	}
	for _, row := range entityTypes {
		r.entityTypeIDs[domain.NewEntityType(row.Name)] = EntityTypeID(row.ID)
	}
	for _, row := range eventTypes {
		entityTypeID := EntityTypeID(row.EntityTypeID)
		if r.eventTypeIDs[entityTypeID] == nil {
			r.eventTypeIDs[entityTypeID] = map[domain.EventName]EventTypeID{}
		}
		name := domain.NewEventName(row.Name)
		r.eventTypeIDs[entityTypeID][name] = EventTypeID(row.ID)
		r.eventTypeNames[EventTypeID(row.ID)] = name
	}
	return r, nil
}

func (r *Registry) EntityTypeID(name domain.EntityType) (EntityTypeID, error) {
	id, ok := r.entityTypeIDs[name]
	if !ok {
		return 0, fmt.Errorf("entity type not found: %s", name)
	}
	return id, nil
}

func (r *Registry) EventTypeID(entityTypeID EntityTypeID, name domain.EventName) (EventTypeID, error) {
	id, ok := r.eventTypeIDs[entityTypeID][name]
	if !ok {
		return 0, fmt.Errorf("event type not found: entity_type_id=%d, name=%s", entityTypeID, name)
	}
	return id, nil
}

func (r *Registry) EventTypeName(id EventTypeID) (domain.EventName, error) {
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

// アプリケーションの起動時に呼ぶ。種別が無いと、一意性の確保が「重複」と区別できずに失敗するため、使うときではなく起動時に検出する
func (r *Registry) VerifyUniqueKeyTypes(keyTypes ...domain.UniqueKeyType) error {
	var errs []error
	for _, keyType := range keyTypes {
		if !r.uniqueKeyTypes[keyType.String()] {
			errs = append(errs, fmt.Errorf("unique key type not found: %s", keyType))
		}
	}
	return errors.Join(errs...)
}
