package spell

import "errors"

var (
	ErrInvalidState        = errors.New("spell: invalid state")
	ErrDrawCountTooSmall   = errors.New("spell: draw count must be at least 1")
	ErrMagicPerLapTooSmall = errors.New("spell: magic per lap must be at least 1")
	ErrDiceCountTooSmall   = errors.New("spell: dice count must be at least 2")
	ErrDiceFaceTooSmall    = errors.New("spell: dice face must be at least 1")
	ErrCurseTurnsTooSmall  = errors.New("spell: curse turns must be at least 1")
)
