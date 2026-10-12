package operator

import (
	"encoding/json"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

type sealedEvent interface {
	isOperatorEvent()
}

type eventMarker struct{}

func (eventMarker) isOperatorEvent() {}

type Event interface {
	sealedEvent
	domain.Event[ID]
}

type EventMeta = domain.EventMeta[ID]

// メールアドレスは、初めてのログインで紐づけるときの照合にだけ使う（紐づけた後は (iss, sub) で識別する）。
// ロールと登録した人は、使う場面（core#10）で足す
type RegisteredEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	PublicID    uuid.UUID `json:"public_id"`
	Email       Email     `json:"email"`
}

var registeredEventType = domain.NewEventName("RegisteredEvent")

func (*RegisteredEvent) EventType() domain.EventName {
	return registeredEventType
}

// payload の形を変えるときは上げ、Unmarshal で古い版を読み替える（保存済みのイベントは書き換えないため）
func (*RegisteredEvent) SchemaVersion() int {
	return 1
}

type LinkedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Issuer      Issuer  `json:"issuer"`
	Subject     Subject `json:"subject"`
}

var linkedEventType = domain.NewEventName("LinkedEvent")

func (*LinkedEvent) EventType() domain.EventName {
	return linkedEventType
}

func (*LinkedEvent) SchemaVersion() int {
	return 1
}

func Types() domain.AggregateTypes {
	return domain.AggregateTypes{EntityType: entityType, EventTypes: []domain.EventName{registeredEventType, linkedEventType}}
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
	case EventType(linkedEventType):
		event = &LinkedEvent{EventMeta: meta}
	default:
		return nil, domain.ErrUnknownEventType
	}
	if err := json.Unmarshal(payload, event); err != nil {
		return nil, err
	}
	return event, nil
}
