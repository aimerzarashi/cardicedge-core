package startergrant

import (
	"encoding/json"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
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

// 配る中身（所持カード・デッキ・解放するボード）を、開始のときにマスタから写して持つ。後でマスタが変わっても、この配布は変わらない。
// プレイヤーはほかの集約なので内部 ID で持つ。解放するボードは内部 ID（board.ID）
type StartedEvent struct {
	EventMeta    `json:"-"`
	eventMarker  `json:"-"`
	Player       player.ID    `json:"player"`
	Cards        []card.Count `json:"cards"`
	Decks        []Deck       `json:"decks"`
	UnlockBoards []board.ID   `json:"unlock_boards"`
}

// 所持カードの集約ができたことの記録
type CollectionRecordedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
}

// 解放の集約ができたことの記録
type UnlockRecordedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
}

// デッキの集約ができたことの記録
type DeckRecordedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Number      DeckNumber `json:"number"`
}

// 所持カード・デッキ・解放の記録がそろった
type CompletedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
}

var (
	startedEventType            = domain.NewEventName("StartedEvent")
	collectionRecordedEventType = domain.NewEventName("CollectionRecordedEvent")
	deckRecordedEventType       = domain.NewEventName("DeckRecordedEvent")
	unlockRecordedEventType     = domain.NewEventName("UnlockRecordedEvent")
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

func (*UnlockRecordedEvent) EventType() domain.EventName {
	return unlockRecordedEventType
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

func (*UnlockRecordedEvent) SchemaVersion() int {
	return 1
}

func (*CompletedEvent) SchemaVersion() int {
	return 1
}

func Types() domain.AggregateTypes {
	return domain.AggregateTypes{
		EntityType: entityType,
		EventTypes: []domain.EventName{startedEventType, collectionRecordedEventType, deckRecordedEventType, completedEventType, unlockRecordedEventType},
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
	case EventType(unlockRecordedEventType):
		event = &UnlockRecordedEvent{EventMeta: meta}
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
