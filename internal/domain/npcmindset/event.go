package npcmindset

import (
	"encoding/json"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type sealedEvent interface {
	isNPCMindsetEvent()
}

type eventMarker struct{}

func (eventMarker) isNPCMindsetEvent() {}

type Event interface {
	sealedEvent
	domain.Event[ID]
}

type EventMeta = domain.EventMeta[ID]

// 登録したときの重み（既定の値）を明示して持つ。既定の値を後で変えても、過去の登録のイベントから作る状態が変わらないようにするため
// （NPC のデッキ・カードの登録のイベントと同じ）
type RegisteredEvent struct {
	EventMeta        `json:"-"`
	eventMarker      `json:"-"`
	PublicID         uuid.UUID        `json:"public_id"`
	Name             Name             `json:"name"`
	ActionWeights    ActionWeights    `json:"action_weights"`
	CriterionWeights CriterionWeights `json:"criterion_weights"`
	RegisteredBy     operator.ID      `json:"registered_by"`
}

type RenamedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Name        Name        `json:"name"`
	RenamedBy   operator.ID `json:"renamed_by"`
}

// 変えた後の重みのすべて（差分にしない。重みは 19 個・4 個と少なく、変えた後の値がそのまま読めるため）
type ActionWeightsChangedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Weights     ActionWeights `json:"weights"`
	ChangedBy   operator.ID   `json:"changed_by"`
}

type CriterionWeightsChangedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Weights     CriterionWeights `json:"weights"`
	ChangedBy   operator.ID      `json:"changed_by"`
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
	registeredEventType              = domain.NewEventName("RegisteredEvent")
	renamedEventType                 = domain.NewEventName("RenamedEvent")
	actionWeightsChangedEventType    = domain.NewEventName("ActionWeightsChangedEvent")
	criterionWeightsChangedEventType = domain.NewEventName("CriterionWeightsChangedEvent")
	publishedEventType               = domain.NewEventName("PublishedEvent")
	retiredEventType                 = domain.NewEventName("RetiredEvent")
	deletedEventType                 = domain.NewEventName("DeletedEvent")
)

func (*RegisteredEvent) EventType() domain.EventName {
	return registeredEventType
}

func (*RenamedEvent) EventType() domain.EventName {
	return renamedEventType
}

func (*ActionWeightsChangedEvent) EventType() domain.EventName {
	return actionWeightsChangedEventType
}

func (*CriterionWeightsChangedEvent) EventType() domain.EventName {
	return criterionWeightsChangedEventType
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

func (*ActionWeightsChangedEvent) SchemaVersion() int {
	return 1
}

func (*CriterionWeightsChangedEvent) SchemaVersion() int {
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
			registeredEventType, renamedEventType, actionWeightsChangedEventType, criterionWeightsChangedEventType, publishedEventType, retiredEventType, deletedEventType,
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
	case EventType(actionWeightsChangedEventType):
		event = &ActionWeightsChangedEvent{EventMeta: meta}
	case EventType(criterionWeightsChangedEventType):
		event = &CriterionWeightsChangedEvent{EventMeta: meta}
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
