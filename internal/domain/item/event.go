package item

import (
	"encoding/json"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type sealedEvent interface {
	isItemEvent()
}

type eventMarker struct{}

func (eventMarker) isItemEvent() {}

type Event interface {
	sealedEvent
	domain.Event[ID]
}

type EventMeta = domain.EventMeta[ID]

// 登録した人は、ほかの集約を指すため内部 ID（operator.ID の数値）で持つ
type RegisteredEvent struct {
	EventMeta    `json:"-"`
	eventMarker  `json:"-"`
	PublicID     uuid.UUID   `json:"public_id"`
	Name         card.Name   `json:"name"`
	RegisteredBy operator.ID `json:"registered_by"`
}

var registeredEventType = domain.NewEventName("RegisteredEvent")

func (*RegisteredEvent) EventType() domain.EventName {
	return registeredEventType
}

// payload の形を変えるときは上げ、Unmarshal で古い版を読み替える（保存済みのイベントは書き換えないため）
func (*RegisteredEvent) SchemaVersion() int {
	return 1
}

// 準備中・使用不可から公開中にした
type PublishedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	PublishedBy operator.ID `json:"published_by"`
}

var publishedEventType = domain.NewEventName("PublishedEvent")

func (*PublishedEvent) EventType() domain.EventName {
	return publishedEventType
}

func (*PublishedEvent) SchemaVersion() int {
	return 1
}

// 公開中から使用不可にした
type RetiredEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	RetiredBy   operator.ID `json:"retired_by"`
}

var retiredEventType = domain.NewEventName("RetiredEvent")

func (*RetiredEvent) EventType() domain.EventName {
	return retiredEventType
}

func (*RetiredEvent) SchemaVersion() int {
	return 1
}

func Types() domain.AggregateTypes {
	return domain.AggregateTypes{
		EntityType: entityType,
		EventTypes: []domain.EventName{registeredEventType, publishedEventType, retiredEventType},
	}
}

type EventType domain.EventName

func NewEventType(name domain.EventName) domain.EventType[ID, Event] {
	return EventType(name)
}

func (t EventType) Unmarshal(meta EventMeta, _ int, payload []byte) (Event, error) {
	var event Event
	switch t {
	case EventType(registeredEventType):
		event = &RegisteredEvent{EventMeta: meta}
	case EventType(publishedEventType):
		event = &PublishedEvent{EventMeta: meta}
	case EventType(retiredEventType):
		event = &RetiredEvent{EventMeta: meta}
	default:
		return nil, domain.ErrUnknownEventType
	}
	if err := json.Unmarshal(payload, event); err != nil {
		return nil, err
	}
	return event, nil
}
