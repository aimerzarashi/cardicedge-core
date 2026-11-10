package spell

import "errors"

var (
	ErrInvalidState      = errors.New("spell: invalid state")
	ErrDrawCountTooSmall = errors.New("spell: draw count must be at least 1")
)
