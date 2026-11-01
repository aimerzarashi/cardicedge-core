package domain

import (
	"context"
	"time"
)

// domain_events の列（entity_id・aggregate_version・occurred_at）に入れるため、payload には含めない（json:"-"）
type EventMeta[I ID] struct {
	EntityID         I         `json:"-"`
	AggregateVersion int64     `json:"-"`
	OccurredAt       time.Time `json:"-"`
}

func (m EventMeta[I]) GetMeta() EventMeta[I] {
	return m
}

type Event[I ID] interface {
	GetMeta() EventMeta[I]
	// 種別名を型名からリフレクションで取り出さず宣言させるのは、型の名前を変えただけで
	// domain_event_types と食い違うのを防ぐため（起動時に domaindef.Verify で照合する）
	EventType() EventName
	// 版をイベントの型に持たせるのは、Decide が保存の都合（版）を気にせずに済むようにするため
	SchemaVersion() int
}

// E を集約固有の Event 型に固定するのは、復元したイベントを型アサーションなしで受け取るため。
// schemaVersion は保存されたときの版で、古い版の payload を読み替えるために渡す
type EventType[I ID, E Event[I]] interface {
	Unmarshal(meta EventMeta[I], schemaVersion int, payload []byte) (E, error)
}

type NewEventTypeFunc[I ID, E Event[I]] func(EventName) EventType[I, E]

// イベントの種別名（domain_event_types の名前）。各集約だけが生成関数で宣言する（EntityType と同じ理由で string にしない）
type EventName string

func NewEventName(name string) EventName {
	return EventName(name)
}

func (n EventName) String() string {
	return string(n)
}

type InitialStateFunc[S any] func() S

type EvolveFunc[S any, E any] func(S, E) (S, error)

type EventStore[I ID, E Event[I], S any] interface {
	Save(context.Context, []E) error
	Load(context.Context, I) (S, error)
}

// AggregateTypes は集約が宣言する種別名とイベント種別名。起動時に domain_entity_types・domain_event_types と照合し、
// マイグレーションの足し忘れや名前の食い違いを、使うときではなく起動時に検出する（設計メモ 4 節の fail-fast）
type AggregateTypes struct {
	EntityType EntityType
	EventTypes []EventName
}
