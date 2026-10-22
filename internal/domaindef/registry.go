package domaindef

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// Registry は domain_entity_types・domain_event_types・domain_unique_key_types・domain_reference_types の name と id の対応。
// パッケージの変数にせず、Load の結果を identifier・eventstore に渡すのは、読み込み忘れをコンパイル時にわかるようにし、
// 依存を引数に表すため（参照の api はパッケージの変数だった）。起動後は読み取り専用のため錠を持たない
type Registry struct {
	entityTypeIDs   map[domain.EntityType]EntityTypeID
	entityTypeNames map[EntityTypeID]domain.EntityType
	eventTypeIDs    map[EntityTypeID]map[domain.EventName]EventTypeID // イベント種別名は集約の中でだけ一意なため、集約ごとに分ける
	eventTypeNames  map[EventTypeID]domain.EventName
	uniqueKeyTypes  map[string]bool
	referenceTypes  map[string]bool
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
	referenceTypes, err := models.DomainReferenceTypes().All(ctx, exec)
	if err != nil {
		return nil, fmt.Errorf("failed to load domain_reference_types: %w", err)
	}

	r := &Registry{
		entityTypeIDs:   map[domain.EntityType]EntityTypeID{},
		entityTypeNames: map[EntityTypeID]domain.EntityType{},
		eventTypeIDs:    map[EntityTypeID]map[domain.EventName]EventTypeID{},
		eventTypeNames:  map[EventTypeID]domain.EventName{},
		uniqueKeyTypes:  map[string]bool{},
		referenceTypes:  map[string]bool{},
	}
	for _, row := range uniqueKeyTypes {
		r.uniqueKeyTypes[row.Name] = true
	}
	for _, row := range referenceTypes {
		r.referenceTypes[row.Name] = true
	}
	for _, row := range entityTypes {
		r.entityTypeIDs[domain.NewEntityType(row.Name)] = EntityTypeID(row.ID)
		r.entityTypeNames[EntityTypeID(row.ID)] = domain.NewEntityType(row.Name)
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

func (r *Registry) EntityTypeName(id EntityTypeID) (domain.EntityType, error) {
	name, ok := r.entityTypeNames[id]
	if !ok {
		return "", fmt.Errorf("entity type id not found: %d", id)
	}
	return name, nil
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

// 起動時の照合（Verify...）は、コードの宣言と DB（マイグレーション）が両方向で一致するかを確かめる。コード → DB の食い違い
// （マイグレーションの足し忘れ・名前の食い違い）は、使うときに初めて失敗するため。DB → コードの食い違い（コードの足し忘れ・
// 型を消した・名前を変えた）は、過去のイベントを読むときや、種別を引くときに初めて失敗するため（core#20）。
// 将来、複数台を順に入れ替えるデプロイにすると、新しいマイグレーションと古いコードが重なる間に止まるため、そのときに考え直す（core#19）

// アプリケーションの起動時に呼ぶ。集約の根だけでなく、識別子を発行する子の entity の種別も渡す
func (r *Registry) VerifyEntityTypes(entityTypes ...domain.EntityType) error {
	declared := make(map[domain.EntityType]bool, len(entityTypes))
	var errs []error
	for _, entityType := range entityTypes {
		declared[entityType] = true
		if _, ok := r.entityTypeIDs[entityType]; !ok {
			errs = append(errs, fmt.Errorf("entity type is not in domain_entity_types: %s", entityType))
		}
	}
	for _, entityType := range sortedKeys(r.entityTypeIDs) {
		if !declared[entityType] {
			errs = append(errs, fmt.Errorf("domain_entity_types has an entity type not declared in code: %s", entityType))
		}
	}
	return errors.Join(errs...)
}

// アプリケーションの起動時に呼ぶ。イベントの種別名は集約の中でだけ一意なため、集約ごとの宣言を渡す
func (r *Registry) VerifyEventTypes(aggregates ...domain.AggregateTypes) error {
	declared := make(map[domain.EntityType]map[domain.EventName]bool, len(aggregates))
	var errs []error
	for _, aggregate := range aggregates {
		declared[aggregate.EntityType] = make(map[domain.EventName]bool, len(aggregate.EventTypes))
		entityTypeID, ok := r.entityTypeIDs[aggregate.EntityType]
		for _, name := range aggregate.EventTypes {
			declared[aggregate.EntityType][name] = true
			if _, found := r.eventTypeIDs[entityTypeID][name]; !ok || !found {
				errs = append(errs, fmt.Errorf("event type is not in domain_event_types: %s.%s", aggregate.EntityType, name))
			}
		}
	}
	for _, entityTypeID := range sortedKeys(r.eventTypeIDs) {
		entityType := r.entityTypeNames[entityTypeID]
		for _, name := range sortedKeys(r.eventTypeIDs[entityTypeID]) {
			if !declared[entityType][name] {
				errs = append(errs, fmt.Errorf("domain_event_types has an event type not declared in code: %s.%s", entityType, name))
			}
		}
	}
	return errors.Join(errs...)
}

// アプリケーションの起動時に呼ぶ。種別が無いと、一意性の確保が「重複」と区別できずに失敗するため、使うときではなく起動時に検出する
func (r *Registry) VerifyUniqueKeyTypes(keyTypes ...domain.UniqueKeyType) error {
	names := make([]string, 0, len(keyTypes))
	for _, keyType := range keyTypes {
		names = append(names, keyType.String())
	}
	return verifyNames("domain_unique_key_types", "unique key type", names, r.uniqueKeyTypes)
}

// アプリケーションの起動時に呼ぶ。種別が無いと参照を足せず、使うときに初めて失敗するため、起動時に検出する
func (r *Registry) VerifyReferenceTypes(referenceTypes ...domain.ReferenceType) error {
	names := make([]string, 0, len(referenceTypes))
	for _, referenceType := range referenceTypes {
		names = append(names, referenceType.String())
	}
	return verifyNames("domain_reference_types", "reference type", names, r.referenceTypes)
}

func verifyNames(table, kind string, declared []string, inDB map[string]bool) error {
	declaredSet := make(map[string]bool, len(declared))
	var errs []error
	for _, name := range declared {
		declaredSet[name] = true
		if !inDB[name] {
			errs = append(errs, fmt.Errorf("%s is not in %s: %s", kind, table, name))
		}
	}
	for _, name := range sortedKeys(inDB) {
		if !declaredSet[name] {
			errs = append(errs, fmt.Errorf("%s has a %s not declared in code: %s", table, kind, name))
		}
	}
	return errors.Join(errs...)
}

// 誤りの並びを毎回同じにする（map の順は決まらないため）
func sortedKeys[K cmp.Ordered, V any](m map[K]V) []K {
	return slices.Sorted(maps.Keys(m))
}
