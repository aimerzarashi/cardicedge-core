package presetdeck

import (
	"encoding/json"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type sealedEvent interface {
	isPresetDeckEvent()
}

type eventMarker struct{}

func (eventMarker) isPresetDeckEvent() {}

type Event interface {
	sealedEvent
	domain.Event[ID]
}

type EventMeta = domain.EventMeta[ID]

// 操作した人は、ほかの集約を指すため内部 ID（operator.ID の数値）で持つ（Creature と同じ）
type RegisteredEvent struct {
	EventMeta    `json:"-"`
	eventMarker  `json:"-"`
	PublicID     uuid.UUID   `json:"public_id"`
	Name         Name        `json:"name"`
	RegisteredBy operator.ID `json:"registered_by"`
}

type RenamedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Name        Name        `json:"name"`
	RenamedBy   operator.ID `json:"renamed_by"`
}

// 1 回の操作で 1 つのイベントにし、実際に変わったカードだけを、変えた枚数の差分で持つ（core#24）。
// 変えた後の枚数でなく差分で持つのは、何枚足した・減らしたかを履歴で読めるようにするため。
// カードごとにイベントを分けないのは、操作とイベントを 1 対 1 にし、1 回の操作で版が 1 つだけ進むようにするため
type CardCountsChangedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Changes     []CardCountChange `json:"changes"`
	ChangedBy   operator.ID       `json:"changed_by"`
}

// Delta は 0 以外（変わらないカードはイベントに入れない）。足した結果が 0 になったカードはデッキから外れる
type CardCountChange struct {
	Card  card.Ref `json:"card"`
	Delta int      `json:"delta"`
}

type DeletedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	DeletedBy   operator.ID `json:"deleted_by"`
}

var (
	registeredEventType        = domain.NewEventName("RegisteredEvent")
	renamedEventType           = domain.NewEventName("RenamedEvent")
	cardCountsChangedEventType = domain.NewEventName("CardCountsChangedEvent")
	deletedEventType           = domain.NewEventName("DeletedEvent")
)

func (*RegisteredEvent) EventType() domain.EventName {
	return registeredEventType
}

func (*RenamedEvent) EventType() domain.EventName {
	return renamedEventType
}

func (*CardCountsChangedEvent) EventType() domain.EventName {
	return cardCountsChangedEventType
}

func (*DeletedEvent) EventType() domain.EventName {
	return deletedEventType
}

// payload の形を変えるときは上げ、Unmarshal で古い版を読み替える（保存済みのイベントは書き換えないため）
func (*RegisteredEvent) SchemaVersion() int {
	return 1
}

func (*RenamedEvent) SchemaVersion() int {
	return 1
}

func (*CardCountsChangedEvent) SchemaVersion() int {
	return 1
}

func (*DeletedEvent) SchemaVersion() int {
	return 1
}

func Types() domain.AggregateTypes {
	return domain.AggregateTypes{
		EntityType: entityType,
		EventTypes: []domain.EventName{registeredEventType, renamedEventType, cardCountsChangedEventType, deletedEventType},
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
	case EventType(renamedEventType):
		event = &RenamedEvent{EventMeta: meta}
	case EventType(cardCountsChangedEventType):
		event = &CardCountsChangedEvent{EventMeta: meta}
	case EventType(deletedEventType):
		event = &DeletedEvent{EventMeta: meta}
	default:
		return nil, domain.ErrUnknownEventType
	}
	if err := json.Unmarshal(payload, event); err != nil {
		return nil, err
	}
	return event, nil
}
