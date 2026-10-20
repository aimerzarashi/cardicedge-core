package starterdeckset

import (
	"encoding/json"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type sealedEvent interface {
	isStarterDeckSetEvent()
}

type eventMarker struct{}

func (eventMarker) isStarterDeckSetEvent() {}

type Event interface {
	sealedEvent
	domain.Event[ID]
}

type EventMeta = domain.EventMeta[ID]

// 写しの中身を全部持つ（read model がこのイベントだけから投影できるように）。公開 ID も毎回持つ（初めての設定かどうかで、
// イベントの形を分けないため）
type DecksSetEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	PublicID    uuid.UUID   `json:"public_id"`
	Decks       []Deck      `json:"decks"`
	SetBy       operator.ID `json:"set_by"`
}

var decksSetEventType = domain.NewEventName("DecksSetEvent")

func (*DecksSetEvent) EventType() domain.EventName {
	return decksSetEventType
}

// payload の形を変えるときは上げ、Unmarshal で古い版を読み替える（保存済みのイベントは書き換えないため）
func (*DecksSetEvent) SchemaVersion() int {
	return 1
}

func Types() domain.AggregateTypes {
	return domain.AggregateTypes{EntityType: entityType, EventTypes: []domain.EventName{decksSetEventType}}
}

type EventType domain.EventName

func NewEventType(name domain.EventName) domain.EventType[ID, Event] {
	return EventType(name)
}

func (t EventType) Unmarshal(meta EventMeta, _ int, payload []byte) (Event, error) {
	var event Event
	switch t {
	case EventType(decksSetEventType):
		event = &DecksSetEvent{EventMeta: meta}
	default:
		return nil, domain.ErrUnknownEventType
	}
	if err := json.Unmarshal(payload, event); err != nil {
		return nil, err
	}
	return event, nil
}
