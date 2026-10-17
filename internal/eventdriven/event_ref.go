package eventdriven

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
)

var ErrInvalidPayload = errors.New("eventdriven: invalid notification payload")

// domain_events の 1 行を指す。中身（payload）は持たず、振り分け・順番待ち・処理済みの位置の管理はこれだけで行う
// （中身は処理する直前に読む。通知の中身には大きさの上限があり、イベントの中身は入れないため）
type EventRef struct {
	EntityTypeID     domaindef.EntityTypeID
	EventTypeID      domaindef.EventTypeID
	EntityID         int64
	AggregateVersion int64
	EventID          int64
}

// 通知の中身（entity_type_id:event_type_id:entity_id:aggregate_version:event_id。migrations の notify_domain_event）を読む
func ParseEventRef(payload string) (EventRef, error) {
	parts := strings.Split(payload, ":")
	if len(parts) != 5 {
		return EventRef{}, fmt.Errorf("%w: %q", ErrInvalidPayload, payload)
	}
	entityTypeID, err1 := strconv.ParseInt(parts[0], 10, 16)
	eventTypeID, err2 := strconv.ParseInt(parts[1], 10, 16)
	entityID, err3 := strconv.ParseInt(parts[2], 10, 64)
	aggregateVersion, err4 := strconv.ParseInt(parts[3], 10, 64)
	eventID, err5 := strconv.ParseInt(parts[4], 10, 64)
	if err := errors.Join(err1, err2, err3, err4, err5); err != nil {
		return EventRef{}, fmt.Errorf("%w: %q: %w", ErrInvalidPayload, payload, err)
	}
	return EventRef{
		EntityTypeID:     domaindef.NewEntityTypeID(int16(entityTypeID)),
		EventTypeID:      domaindef.NewEventTypeID(int16(eventTypeID)),
		EntityID:         entityID,
		AggregateVersion: aggregateVersion,
		EventID:          eventID,
	}, nil
}
