package match

import (
	"encoding/json"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

type sealedEvent interface {
	isMatchEvent()
}

type eventMarker struct{}

func (eventMarker) isMatchEvent() {}

type Event interface {
	sealedEvent
	domain.Event[ID]
}

type EventMeta = domain.EventMeta[ID]

// 始めた時点の条件の写し。部屋は持たない（部屋のイベントが対戦の ID を持つので、対戦が部屋を指すとパッケージが循環する。
// 対戦から部屋をたどるのは、結果を知らせる ⑦ で部屋の read model から引く）
type CreatedEvent struct {
	EventMeta    `json:"-"`
	eventMarker  `json:"-"`
	Participants []Participant `json:"participants"`
	Board        Board         `json:"board"`
	Rules        Rules         `json:"rules"`
}

// 無作為に決めた順番（参加者の番号の並び。早い順）。サイコロの振り合いにしないのは、順番を決める場面を画面に出さないため（core#100）
type TurnOrderDecidedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Order       []ParticipantIndex `json:"order"`
}

// 参加者ごとの山札の並び（参加者の順。先頭から引く）。並びは見せないが、読み直しても同じ状態にするため残す
type DecksShuffledEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	DrawPiles   [][]card.Ref `json:"draw_piles"`
}

// 全員が山札の先頭から同じ枚数を手札に引いた（山札が足りなければ、ある分だけ）
type HandsDealtEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Cards       int `json:"cards"`
}

var (
	createdEventType          = domain.NewEventName("CreatedEvent")
	turnOrderDecidedEventType = domain.NewEventName("TurnOrderDecidedEvent")
	decksShuffledEventType    = domain.NewEventName("DecksShuffledEvent")
	handsDealtEventType       = domain.NewEventName("HandsDealtEvent")
)

func (*CreatedEvent) EventType() domain.EventName {
	return createdEventType
}

func (*TurnOrderDecidedEvent) EventType() domain.EventName {
	return turnOrderDecidedEventType
}

func (*DecksShuffledEvent) EventType() domain.EventName {
	return decksShuffledEventType
}

func (*HandsDealtEvent) EventType() domain.EventName {
	return handsDealtEventType
}

// payload の形を変えるときは上げ、Unmarshal で古い版を読み替える（保存済みのイベントは書き換えないため）
func (*CreatedEvent) SchemaVersion() int {
	return 1
}

func (*TurnOrderDecidedEvent) SchemaVersion() int {
	return 1
}

func (*DecksShuffledEvent) SchemaVersion() int {
	return 1
}

func (*HandsDealtEvent) SchemaVersion() int {
	return 1
}

func Types() domain.AggregateTypes {
	return domain.AggregateTypes{EntityType: entityType, EventTypes: []domain.EventName{
		createdEventType, turnOrderDecidedEventType, decksShuffledEventType, handsDealtEventType,
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
	case EventType(turnOrderDecidedEventType):
		event = &TurnOrderDecidedEvent{EventMeta: meta}
	case EventType(decksShuffledEventType):
		event = &DecksShuffledEvent{EventMeta: meta}
	case EventType(handsDealtEventType):
		event = &HandsDealtEvent{EventMeta: meta}
	default:
		return nil, domain.ErrUnknownEventType
	}
	if err := json.Unmarshal(payload, event); err != nil {
		return nil, err
	}
	return event, nil
}
