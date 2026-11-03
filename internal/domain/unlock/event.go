package unlock

import (
	"encoding/json"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

type sealedEvent interface {
	isUnlockEvent()
}

type eventMarker struct{}

func (eventMarker) isUnlockEvent() {}

type Event interface {
	sealedEvent
	domain.Event[ID]
}

type EventMeta = domain.EventMeta[ID]

// 持ち主は、ほかの集約なので内部 ID で持つ。解放の理由や配布の ID は持たない（実績など、解放のきっかけが増えても形を変えないため）
type CreatedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Player      player.ID `json:"player"`
}

// 新しく解放したボード（1 つ以上。すでに解放していたものは含まない）。解放した順。
// ボードは内部 ID（集約はほかの集約を内部 ID で指す。core_game には publish-master が studio の内部 ID も写す）
type BoardsUnlockedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Boards      []board.ID `json:"boards"`
}

var (
	createdEventType        = domain.NewEventName("CreatedEvent")
	boardsUnlockedEventType = domain.NewEventName("BoardsUnlockedEvent")
)

func (*CreatedEvent) EventType() domain.EventName {
	return createdEventType
}

func (*BoardsUnlockedEvent) EventType() domain.EventName {
	return boardsUnlockedEventType
}

// payload の形を変えるときは上げ、Unmarshal で古い版を読み替える（保存済みのイベントは書き換えないため）
func (*CreatedEvent) SchemaVersion() int {
	return 1
}

func (*BoardsUnlockedEvent) SchemaVersion() int {
	return 1
}

func Types() domain.AggregateTypes {
	return domain.AggregateTypes{EntityType: entityType, EventTypes: []domain.EventName{createdEventType, boardsUnlockedEventType}}
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
	case EventType(boardsUnlockedEventType):
		event = &BoardsUnlockedEvent{EventMeta: meta}
	default:
		return nil, domain.ErrUnknownEventType
	}
	if err := json.Unmarshal(payload, event); err != nil {
		return nil, err
	}
	return event, nil
}
