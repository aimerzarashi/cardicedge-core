package domain

import "errors"

var (
	ErrUnknownEventType = errors.New("unknown event type")
	ErrVersionConflict  = errors.New("aggregate version conflict")
)
