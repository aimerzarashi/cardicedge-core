package grantdeck

import (
	"encoding/json"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

type sealedEvent interface {
	isGrantDeckEvent()
}

type eventMarker struct{}

func (eventMarker) isGrantDeckEvent() {}

type Event interface {
	sealedEvent
	domain.Event[ID]
}

type EventMeta = domain.EventMeta[ID]

// プリセットデッキから写して作ったときだけ、写した元とカードを持つ。写した中身を登録のイベントに入れる（別のイベントにしない）のは、
// 登録と写しを 1 回の操作・1 つの版にし、read model がこのイベントだけから投影できるようにするため。
// 操作した人はほかの集約を指すため内部 ID で持つ
type RegisteredEvent struct {
	EventMeta    `json:"-"`
	eventMarker  `json:"-"`
	PublicID     uuid.UUID       `json:"public_id"`
	Name         presetdeck.Name `json:"name"`
	Source       *Source         `json:"source,omitempty"`
	Cards        []DeckCard      `json:"cards,omitempty"`
	RegisteredBy operator.ID     `json:"registered_by"`
}

// 写したときのカードと枚数。枚数は 1 以上
type DeckCard struct {
	Card     card.Ref          `json:"card"`
	Quantity card.DeckQuantity `json:"quantity"`
}

type RenamedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Name        presetdeck.Name `json:"name"`
	RenamedBy   operator.ID     `json:"renamed_by"`
}

// 1 回の操作で 1 つのイベントにし、実際に変わったカードだけを、変えた枚数の差分で持つ（presetdeck.CardCountsChangedEvent と同じ）
type CardCountsChangedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Changes     []CardCountChange `json:"changes"`
	ChangedBy   operator.ID       `json:"changed_by"`
}

// Delta は 0 以外。足した結果が 0 になったカードはデッキから外れる
type CardCountChange struct {
	Card  card.Ref `json:"card"`
	Delta int      `json:"delta"`
}

type PublishedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	PublishedBy operator.ID `json:"published_by"`
}

type RetiredEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	RetiredBy   operator.ID `json:"retired_by"`
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
	publishedEventType         = domain.NewEventName("PublishedEvent")
	retiredEventType           = domain.NewEventName("RetiredEvent")
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

func (*PublishedEvent) EventType() domain.EventName {
	return publishedEventType
}

func (*RetiredEvent) EventType() domain.EventName {
	return retiredEventType
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

func (*PublishedEvent) SchemaVersion() int {
	return 1
}

func (*RetiredEvent) SchemaVersion() int {
	return 1
}

func (*DeletedEvent) SchemaVersion() int {
	return 1
}

func Types() domain.AggregateTypes {
	return domain.AggregateTypes{
		EntityType: entityType,
		EventTypes: []domain.EventName{
			registeredEventType, renamedEventType, cardCountsChangedEventType, publishedEventType, retiredEventType, deletedEventType,
		},
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
	case EventType(publishedEventType):
		event = &PublishedEvent{EventMeta: meta}
	case EventType(retiredEventType):
		event = &RetiredEvent{EventMeta: meta}
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
