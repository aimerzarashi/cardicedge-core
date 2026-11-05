package collection

import (
	"encoding/json"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
)

type sealedEvent interface {
	isCollectionEvent()
}

type eventMarker struct{}

func (eventMarker) isCollectionEvent() {}

type Event interface {
	sealedEvent
	domain.Event[ID]
}

type EventMeta = domain.EventMeta[ID]

// 持ち主と配布は、ほかの集約なので内部 ID で持つ。カードは game の内部 ID
type CreatedEvent struct {
	EventMeta    `json:"-"`
	eventMarker  `json:"-"`
	Player       player.ID       `json:"player"`
	Cards        []Card          `json:"cards"`
	StarterGrant startergrant.ID `json:"starter_grant"`
}

// 所持品に足したカードと、その出どころ。足すだけで、枚数の合計は持たない
type CardsAcquiredEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Cards       []Card `json:"cards"`
	Source      Source `json:"source"`
}

var (
	createdEventType       = domain.NewEventName("CreatedEvent")
	cardsAcquiredEventType = domain.NewEventName("CardsAcquiredEvent")
)

func (*CardsAcquiredEvent) EventType() domain.EventName {
	return cardsAcquiredEventType
}

func (*CardsAcquiredEvent) SchemaVersion() int {
	return 1
}

func (*CreatedEvent) EventType() domain.EventName {
	return createdEventType
}

// payload の形を変えるときは上げ、Unmarshal で古い版を読み替える（保存済みのイベントは書き換えないため）
func (*CreatedEvent) SchemaVersion() int {
	return 1
}

func Types() domain.AggregateTypes {
	return domain.AggregateTypes{EntityType: entityType, EventTypes: []domain.EventName{createdEventType, cardsAcquiredEventType}}
}

type EventType domain.EventName

func NewEventType(name domain.EventName) domain.EventType[ID, Event] {
	return EventType(name)
}

func (t EventType) Unmarshal(meta EventMeta, _ int, payload []byte) (Event, error) {
	var event Event
	switch t {
	case EventType(createdEventType):
		event = &CreatedEvent{EventMeta: meta}
	case EventType(cardsAcquiredEventType):
		event = &CardsAcquiredEvent{EventMeta: meta}
	default:
		return nil, domain.ErrUnknownEventType
	}
	if err := json.Unmarshal(payload, event); err != nil {
		return nil, err
	}
	return event, nil
}
