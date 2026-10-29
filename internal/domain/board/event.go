package board

import (
	"encoding/json"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type sealedEvent interface {
	isBoardEvent()
}

type eventMarker struct{}

func (eventMarker) isBoardEvent() {}

type Event interface {
	sealedEvent
	domain.Event[ID]
}

type EventMeta = domain.EventMeta[ID]

// 登録の直後の大きさと既定のルールを明示的に持つ。DefaultSize や既定の値の定数を変えても、過去のボードの意味が変わらないため
type RegisteredEvent struct {
	EventMeta    `json:"-"`
	eventMarker  `json:"-"`
	PublicID     uuid.UUID    `json:"public_id"`
	Name         Name         `json:"name"`
	Width        int          `json:"width"`
	Height       int          `json:"height"`
	DefaultRules DefaultRules `json:"default_rules"`
	RegisteredBy operator.ID  `json:"registered_by"`
}

type RenamedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Name        Name        `json:"name"`
	RenamedBy   operator.ID `json:"renamed_by"`
}

// 1 回の操作で 1 つのイベントにし、前のレイアウトとの差分だけを持つ（presetdeck.CardCountsChangedEvent と同じ）。
// 属性もマスの一部なので、同じ座標で種類か属性が変わったら ChangedTiles に新しい種類と属性を入れる。
// どの一覧も y → x（つながりは a → b）の順で、空なら省く。マスを消して切れたつながりも Disconnected に入る
// （前と後のつながりの集合の差なので、Evolve が消したマスのつながりを別に探さなくてよい）
type LayoutChangedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	// 大きさが変わったときだけ
	Size         *Size        `json:"size,omitempty"`
	PlacedTiles  []Tile       `json:"placed_tiles,omitempty"`
	RemovedTiles []Position   `json:"removed_tiles,omitempty"`
	ChangedTiles []Tile       `json:"changed_tiles,omitempty"`
	Connected    []Connection `json:"connected,omitempty"`
	Disconnected []Connection `json:"disconnected,omitempty"`
	ChangedBy    operator.ID  `json:"changed_by"`
}

// 変えた後の全体を持つ。値が 4 つと小さいので、レイアウトのような差分にしない
type DefaultRulesChangedEvent struct {
	EventMeta    `json:"-"`
	eventMarker  `json:"-"`
	DefaultRules DefaultRules `json:"default_rules"`
	ChangedBy    operator.ID  `json:"changed_by"`
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
	registeredEventType          = domain.NewEventName("RegisteredEvent")
	renamedEventType             = domain.NewEventName("RenamedEvent")
	layoutChangedEventType       = domain.NewEventName("LayoutChangedEvent")
	publishedEventType           = domain.NewEventName("PublishedEvent")
	defaultRulesChangedEventType = domain.NewEventName("DefaultRulesChangedEvent")
	retiredEventType             = domain.NewEventName("RetiredEvent")
	deletedEventType             = domain.NewEventName("DeletedEvent")
)

func (*RegisteredEvent) EventType() domain.EventName {
	return registeredEventType
}

func (*RenamedEvent) EventType() domain.EventName {
	return renamedEventType
}

func (*LayoutChangedEvent) EventType() domain.EventName {
	return layoutChangedEventType
}

func (*DefaultRulesChangedEvent) EventType() domain.EventName {
	return defaultRulesChangedEventType
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

func (*LayoutChangedEvent) SchemaVersion() int {
	return 1
}

func (*DefaultRulesChangedEvent) SchemaVersion() int {
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
			registeredEventType, renamedEventType, layoutChangedEventType, publishedEventType, retiredEventType, deletedEventType, defaultRulesChangedEventType,
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
	case EventType(layoutChangedEventType):
		event = &LayoutChangedEvent{EventMeta: meta}
	case EventType(defaultRulesChangedEventType):
		event = &DefaultRulesChangedEvent{EventMeta: meta}
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
