package room

import (
	"encoding/json"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

type sealedEvent interface {
	isRoomEvent()
}

type eventMarker struct{}

func (eventMarker) isRoomEvent() {}

type Event interface {
	sealedEvent
	domain.Event[ID]
}

type EventMeta = domain.EventMeta[ID]

// 持ち主は、ほかの集約なので内部 ID で持つ。作った人はプレイヤーの席に自動で座る（席の並びを決めるまでは席が 0）
type CreatedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Player      player.ID `json:"player"`
	Name        Name      `json:"name"`
}

// 変えた後の席の並びの全体。席ごとの選択はすべて外れる
type SeatsChangedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Seats       []Seat `json:"seats"`
}

// 変えた後の対戦ルールの全体
type MatchRulesChangedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	MatchRules  MatchRules `json:"match_rules"`
}

// 席の選択を、変えた後の値にする。選んだ人は、プレイヤーの席は座っている人、NPC の席は作った人
type SeatChoiceChangedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Seat        SeatIndex  `json:"seat"`
	Choice      SeatChoice `json:"choice"`
	ChangedBy   player.ID  `json:"changed_by"`
}

// 公開した。この時点で決まっていたものは、準備中に変えられなくなる。定員なら続けて PreparationStartedEvent が出る
type PublishedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
}

// プレイヤーの席が定員になり、準備中に進んだ（全員の準備完了まで）
type PreparationStartedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
}

// 準備完了を申告した人（自分の担当の分を選び直せなくなる）
type ReadyDeclaredEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Player      player.ID `json:"player"`
}

type ClosedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
}

// 変えた後の名前
type RenamedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Name        Name `json:"name"`
}

var (
	createdEventType            = domain.NewEventName("CreatedEvent")
	seatsChangedEventType       = domain.NewEventName("SeatsChangedEvent")
	matchRulesChangedEventType  = domain.NewEventName("MatchRulesChangedEvent")
	seatChoiceChangedEventType  = domain.NewEventName("SeatChoiceChangedEvent")
	publishedEventType          = domain.NewEventName("PublishedEvent")
	preparationStartedEventType = domain.NewEventName("PreparationStartedEvent")
	readyDeclaredEventType      = domain.NewEventName("ReadyDeclaredEvent")
	closedEventType             = domain.NewEventName("ClosedEvent")
	renamedEventType            = domain.NewEventName("RenamedEvent")
)

func (*CreatedEvent) EventType() domain.EventName {
	return createdEventType
}

func (*SeatsChangedEvent) EventType() domain.EventName {
	return seatsChangedEventType
}

func (*MatchRulesChangedEvent) EventType() domain.EventName {
	return matchRulesChangedEventType
}

func (*SeatChoiceChangedEvent) EventType() domain.EventName {
	return seatChoiceChangedEventType
}

func (*PublishedEvent) EventType() domain.EventName {
	return publishedEventType
}

func (*PreparationStartedEvent) EventType() domain.EventName {
	return preparationStartedEventType
}

func (*ReadyDeclaredEvent) EventType() domain.EventName {
	return readyDeclaredEventType
}

func (*ClosedEvent) EventType() domain.EventName {
	return closedEventType
}

func (*RenamedEvent) EventType() domain.EventName {
	return renamedEventType
}

// payload の形を変えるときは上げ、Unmarshal で古い版を読み替える（保存済みのイベントは書き換えないため）
func (*CreatedEvent) SchemaVersion() int {
	return 1
}

func (*SeatsChangedEvent) SchemaVersion() int {
	return 1
}

func (*MatchRulesChangedEvent) SchemaVersion() int {
	return 1
}

func (*SeatChoiceChangedEvent) SchemaVersion() int {
	return 1
}

func (*PublishedEvent) SchemaVersion() int {
	return 1
}

func (*PreparationStartedEvent) SchemaVersion() int {
	return 1
}

func (*ReadyDeclaredEvent) SchemaVersion() int {
	return 1
}

func (*ClosedEvent) SchemaVersion() int {
	return 1
}

func (*RenamedEvent) SchemaVersion() int {
	return 1
}

func Types() domain.AggregateTypes {
	return domain.AggregateTypes{EntityType: entityType, EventTypes: []domain.EventName{
		createdEventType, seatsChangedEventType, matchRulesChangedEventType, seatChoiceChangedEventType,
		publishedEventType, preparationStartedEventType, readyDeclaredEventType, closedEventType, renamedEventType,
	}}
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
	case EventType(seatsChangedEventType):
		event = &SeatsChangedEvent{EventMeta: meta}
	case EventType(matchRulesChangedEventType):
		event = &MatchRulesChangedEvent{EventMeta: meta}
	case EventType(seatChoiceChangedEventType):
		event = &SeatChoiceChangedEvent{EventMeta: meta}
	case EventType(publishedEventType):
		event = &PublishedEvent{EventMeta: meta}
	case EventType(preparationStartedEventType):
		event = &PreparationStartedEvent{EventMeta: meta}
	case EventType(readyDeclaredEventType):
		event = &ReadyDeclaredEvent{EventMeta: meta}
	case EventType(closedEventType):
		event = &ClosedEvent{EventMeta: meta}
	case EventType(renamedEventType):
		event = &RenamedEvent{EventMeta: meta}
	default:
		return nil, domain.ErrUnknownEventType
	}
	if err := json.Unmarshal(payload, event); err != nil {
		return nil, err
	}
	return event, nil
}
