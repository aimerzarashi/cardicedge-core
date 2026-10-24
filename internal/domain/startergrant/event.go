package startergrant

import (
	"encoding/json"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

type sealedEvent interface {
	isStarterGrantEvent()
}

type eventMarker struct{}

func (eventMarker) isStarterGrantEvent() {}

type Event interface {
	sealedEvent
	domain.Event[ID]
}

type EventMeta = domain.EventMeta[ID]

// 配る中身（所持カードとデッキ）を、開始のときにマスタから写して持つ。後でマスタが変わっても、この配布は変わらない。
// プレイヤーはほかの集約なので内部 ID で持つ
type StartedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Player      player.ID   `json:"player"`
	Cards       []CardCount `json:"cards"`
	Decks       []Deck      `json:"decks"`
}

// 所持カードの集約ができたことの記録
type CollectionRecordedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
}

// デッキの集約ができたことの記録
type DeckRecordedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Number      DeckNumber `json:"number"`
}

// 所持カードとデッキの記録がそろった
type CompletedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
}

var (
	startedEventType            = domain.NewEventName("StartedEvent")
	collectionRecordedEventType = domain.NewEventName("CollectionRecordedEvent")
	deckRecordedEventType       = domain.NewEventName("DeckRecordedEvent")
	completedEventType          = domain.NewEventName("CompletedEvent")
)

func (*StartedEvent) EventType() domain.EventName {
	return startedEventType
}

func (*CollectionRecordedEvent) EventType() domain.EventName {
	return collectionRecordedEventType
}

func (*DeckRecordedEvent) EventType() domain.EventName {
	return deckRecordedEventType
}

func (*CompletedEvent) EventType() domain.EventName {
	return completedEventType
}

// payload の形を変えるときは上げ、Unmarshal で古い版を読み替える（保存済みのイベントは書き換えないため）
func (*StartedEvent) SchemaVersion() int {
	return 1
}

func (*CollectionRecordedEvent) SchemaVersion() int {
	return 1
}

func (*DeckRecordedEvent) SchemaVersion() int {
	return 1
}

func (*CompletedEvent) SchemaVersion() int {
	return 1
}

func Types() domain.AggregateTypes {
	return domain.AggregateTypes{
		EntityType: entityType,
		EventTypes: []domain.EventName{startedEventType, collectionRecordedEventType, deckRecordedEventType, completedEventType},
	}
}

type EventType domain.EventName

func NewEventType(name domain.EventName) domain.EventType[ID, Event] {
	return EventType(name)
}

func (t EventType) Unmarshal(meta EventMeta, _ int, payload []byte) (Event, error) {
	var event Event
	switch t {
	case EventType(startedEventType):
		event = &StartedEvent{EventMeta: meta}
	case EventType(collectionRecordedEventType):
		event = &CollectionRecordedEvent{EventMeta: meta}
	case EventType(deckRecordedEventType):
		event = &DeckRecordedEvent{EventMeta: meta}
	case EventType(completedEventType):
		event = &CompletedEvent{EventMeta: meta}
	default:
		return nil, domain.ErrUnknownEventType
	}
	if err := json.Unmarshal(payload, event); err != nil {
		return nil, err
	}
	return event, nil
}
