package eventstore

import (
	"context"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
)

type LoadedEvent struct {
	Event            any
	AggregateVersion int64
	// 以下は処理のログ（eventdriven）が、保存のログと同じ項目でイベントを引けるようにするためのもの
	EventID    int64
	EventType  domain.EventName
	OccurredAt time.Time
}

// 投影（internal/eventdriven）は、集約の型を知らずに、知らせの entity_id から集約のイベントを読み直す。
// そのため集約の型を消して渡す。domain_events を読む SQL と復元は eventstore の中だけに置き、Load と共有する
// （参照の api は投影が SQL を自分で書くが、読み方が 2 か所に分かれるため採らない）
type EventLoader interface {
	EntityType() domain.EntityType
	// 版の順にすべて返す。1 件でも復元できなければエラーにする（欠けた状態で read model を書かないため）
	LoadEvents(ctx context.Context, entityID int64) ([]LoadedEvent, error)
}

func NewEventLoader[I domain.ID, E domain.Event[I]](
	registry *domaindef.Registry,
	newID domain.NewIDFunc[I],
	newEventType domain.NewEventTypeFunc[I, E],
) EventLoader {
	return &eventLoader[I, E]{reader: reader[I, E]{registry: registry, newID: newID, newEventType: newEventType}}
}

type eventLoader[I domain.ID, E domain.Event[I]] struct {
	reader[I, E]
}

func (l *eventLoader[I, E]) EntityType() domain.EntityType {
	var zero I
	return zero.EntityType()
}

func (l *eventLoader[I, E]) LoadEvents(ctx context.Context, entityID int64) ([]LoadedEvent, error) {
	id, err := l.newID(entityID)
	if err != nil {
		return nil, err
	}
	events, err := l.loadEvents(ctx, id)
	if err != nil {
		return nil, err
	}
	loaded := make([]LoadedEvent, 0, len(events))
	for _, decoded := range events {
		loaded = append(loaded, LoadedEvent{
			Event: decoded.event, AggregateVersion: decoded.event.GetMeta().AggregateVersion,
			EventID: decoded.row.ID, EventType: decoded.event.EventType(), OccurredAt: decoded.event.GetMeta().OccurredAt,
		})
	}
	return loaded, nil
}
