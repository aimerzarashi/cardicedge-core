package match

import (
	"encoding/json"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
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

// 引いたカード。山札の先頭と同じはず（読み直しで食い違えば不整合）
type CardDrawnEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Card        card.Ref         `json:"card"`
}

// 手札から捨てたカード（手札の何番目か。0 始まり）
type CardDiscardedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	HandIndex   int              `json:"hand_index"`
	Card        card.Ref         `json:"card"`
}

// 振った目
type DiceRolledEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Value       int              `json:"value"`
}

// 1 歩進んで入ったマス
type MovedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	To          board.Position   `json:"to"`
}

// まだ印の無いチェックポイントに入って印を付け、魔力が増えた（Magic は足した量）
type CheckpointPassedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Checkpoint  board.Position   `json:"checkpoint"`
	Magic       int              `json:"magic"`
}

// 全チェックポイントに印を付けて拠点に入り、印を外して魔力が増えた（Magic は足した量）
type LapCompletedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Magic       int              `json:"magic"`
}

// 今の番の人がターンを終えた（次の人・ラウンドは Evolve が決める）
type TurnEndedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
}

// 対戦が終わった
type MatchEndedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Winners     []ParticipantIndex `json:"winners"`
	Reason      EndReason          `json:"reason"`
}

var (
	createdEventType          = domain.NewEventName("CreatedEvent")
	turnOrderDecidedEventType = domain.NewEventName("TurnOrderDecidedEvent")
	decksShuffledEventType    = domain.NewEventName("DecksShuffledEvent")
	handsDealtEventType       = domain.NewEventName("HandsDealtEvent")
	cardDrawnEventType        = domain.NewEventName("CardDrawnEvent")
	cardDiscardedEventType    = domain.NewEventName("CardDiscardedEvent")
	diceRolledEventType       = domain.NewEventName("DiceRolledEvent")
	movedEventType            = domain.NewEventName("MovedEvent")
	checkpointPassedEventType = domain.NewEventName("CheckpointPassedEvent")
	lapCompletedEventType     = domain.NewEventName("LapCompletedEvent")
	turnEndedEventType        = domain.NewEventName("TurnEndedEvent")
	matchEndedEventType       = domain.NewEventName("MatchEndedEvent")
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

func (*CardDrawnEvent) EventType() domain.EventName {
	return cardDrawnEventType
}

func (*CardDrawnEvent) SchemaVersion() int {
	return 1
}

func (*CardDiscardedEvent) EventType() domain.EventName {
	return cardDiscardedEventType
}

func (*CardDiscardedEvent) SchemaVersion() int {
	return 1
}

func (*DiceRolledEvent) EventType() domain.EventName {
	return diceRolledEventType
}

func (*DiceRolledEvent) SchemaVersion() int {
	return 1
}

func (*MovedEvent) EventType() domain.EventName {
	return movedEventType
}

func (*MovedEvent) SchemaVersion() int {
	return 1
}

func (*CheckpointPassedEvent) EventType() domain.EventName {
	return checkpointPassedEventType
}

func (*CheckpointPassedEvent) SchemaVersion() int {
	return 1
}

func (*LapCompletedEvent) EventType() domain.EventName {
	return lapCompletedEventType
}

func (*LapCompletedEvent) SchemaVersion() int {
	return 1
}

func (*TurnEndedEvent) EventType() domain.EventName {
	return turnEndedEventType
}

func (*TurnEndedEvent) SchemaVersion() int {
	return 1
}

func (*MatchEndedEvent) EventType() domain.EventName {
	return matchEndedEventType
}

func (*MatchEndedEvent) SchemaVersion() int {
	return 1
}

func Types() domain.AggregateTypes {
	return domain.AggregateTypes{EntityType: entityType, EventTypes: []domain.EventName{
		createdEventType, turnOrderDecidedEventType, decksShuffledEventType, handsDealtEventType,
		cardDrawnEventType, cardDiscardedEventType, diceRolledEventType, movedEventType, checkpointPassedEventType, lapCompletedEventType, turnEndedEventType, matchEndedEventType,
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
	case EventType(cardDrawnEventType):
		event = &CardDrawnEvent{EventMeta: meta}
	case EventType(cardDiscardedEventType):
		event = &CardDiscardedEvent{EventMeta: meta}
	case EventType(diceRolledEventType):
		event = &DiceRolledEvent{EventMeta: meta}
	case EventType(movedEventType):
		event = &MovedEvent{EventMeta: meta}
	case EventType(checkpointPassedEventType):
		event = &CheckpointPassedEvent{EventMeta: meta}
	case EventType(lapCompletedEventType):
		event = &LapCompletedEvent{EventMeta: meta}
	case EventType(turnEndedEventType):
		event = &TurnEndedEvent{EventMeta: meta}
	case EventType(matchEndedEventType):
		event = &MatchEndedEvent{EventMeta: meta}
	default:
		return nil, domain.ErrUnknownEventType
	}
	if err := json.Unmarshal(payload, event); err != nil {
		return nil, err
	}
	return event, nil
}
