package starterset

import (
	"encoding/json"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type sealedEvent interface {
	isStarterSetEvent()
}

type eventMarker struct{}

func (eventMarker) isStarterSetEvent() {}

type Event interface {
	sealedEvent
	domain.Event[ID]
}

type EventMeta = domain.EventMeta[ID]

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

// 配るデッキに割り当てたもの（今の一覧に無かったもの）。一覧の後ろに足す（一覧の順は割り当てた順）。
// 割り当てと割り当て解除をイベントに分けるのは、起きたこと（どのデッキを割り当てた・外した）をそのまま残し、前後の一覧を
// 見比べずに読めるようにするため。配布デッキは別の集約を指すため内部 ID で持つ
type GrantDecksAssignedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	GrantDecks  []grantdeck.ID `json:"grant_decks"`
	AssignedBy  operator.ID    `json:"assigned_by"`
}

// 配るデッキから外したもの（今の一覧にあったもの）
type GrantDecksUnassignedEvent struct {
	EventMeta    `json:"-"`
	eventMarker  `json:"-"`
	GrantDecks   []grantdeck.ID `json:"grant_decks"`
	UnassignedBy operator.ID    `json:"unassigned_by"`
}

// 解放するボードに割り当てたもの。ボードは別の集約を指すため内部 ID で持つ
type UnlockBoardsAssignedEvent struct {
	EventMeta    `json:"-"`
	eventMarker  `json:"-"`
	UnlockBoards []board.ID  `json:"unlock_boards"`
	AssignedBy   operator.ID `json:"assigned_by"`
}

// 解放するボードから外したもの
type UnlockBoardsUnassignedEvent struct {
	EventMeta    `json:"-"`
	eventMarker  `json:"-"`
	UnlockBoards []board.ID  `json:"unlock_boards"`
	UnassignedBy operator.ID `json:"unassigned_by"`
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
	registeredEventType             = domain.NewEventName("RegisteredEvent")
	renamedEventType                = domain.NewEventName("RenamedEvent")
	grantDecksAssignedEventType     = domain.NewEventName("GrantDecksAssignedEvent")
	grantDecksUnassignedEventType   = domain.NewEventName("GrantDecksUnassignedEvent")
	unlockBoardsAssignedEventType   = domain.NewEventName("UnlockBoardsAssignedEvent")
	unlockBoardsUnassignedEventType = domain.NewEventName("UnlockBoardsUnassignedEvent")
	publishedEventType              = domain.NewEventName("PublishedEvent")
	retiredEventType                = domain.NewEventName("RetiredEvent")
	deletedEventType                = domain.NewEventName("DeletedEvent")
)

func (*RegisteredEvent) EventType() domain.EventName {
	return registeredEventType
}

func (*RenamedEvent) EventType() domain.EventName {
	return renamedEventType
}

func (*GrantDecksAssignedEvent) EventType() domain.EventName {
	return grantDecksAssignedEventType
}

func (*GrantDecksUnassignedEvent) EventType() domain.EventName {
	return grantDecksUnassignedEventType
}

func (*UnlockBoardsAssignedEvent) EventType() domain.EventName {
	return unlockBoardsAssignedEventType
}

func (*UnlockBoardsUnassignedEvent) EventType() domain.EventName {
	return unlockBoardsUnassignedEventType
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

func (*GrantDecksAssignedEvent) SchemaVersion() int {
	return 1
}

func (*GrantDecksUnassignedEvent) SchemaVersion() int {
	return 1
}

func (*UnlockBoardsAssignedEvent) SchemaVersion() int {
	return 1
}

func (*UnlockBoardsUnassignedEvent) SchemaVersion() int {
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
			registeredEventType, renamedEventType, deletedEventType, publishedEventType, retiredEventType,
			grantDecksAssignedEventType, grantDecksUnassignedEventType, unlockBoardsAssignedEventType, unlockBoardsUnassignedEventType,
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
	case EventType(grantDecksAssignedEventType):
		event = &GrantDecksAssignedEvent{EventMeta: meta}
	case EventType(grantDecksUnassignedEventType):
		event = &GrantDecksUnassignedEvent{EventMeta: meta}
	case EventType(unlockBoardsAssignedEventType):
		event = &UnlockBoardsAssignedEvent{EventMeta: meta}
	case EventType(unlockBoardsUnassignedEventType):
		event = &UnlockBoardsUnassignedEvent{EventMeta: meta}
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
