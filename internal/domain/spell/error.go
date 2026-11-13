package spell

import "errors"

var (
	ErrInvalidState         = errors.New("spell: invalid state")
	ErrDrawCountTooSmall    = errors.New("spell: draw count must be at least 1")
	ErrMagicPerLapTooSmall  = errors.New("spell: magic per lap must be at least 1")
	ErrDiceCountTooSmall    = errors.New("spell: dice count must be at least 2")
	ErrDiceFaceTooSmall     = errors.New("spell: dice face must be at least 1")
	ErrEffectTurnsTooSmall  = errors.New("spell: effect turns must be at least 1")
	ErrDamageAmountTooSmall = errors.New("spell: damage amount must be at least 1")
	// 効果の無いスペルは公開できない
	ErrNoEffect = errors.New("spell: no effect")
)
