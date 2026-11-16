package effect

import "errors"

var (
	ErrAttackOverrideNegative          = errors.New("effect: attack override must not be negative")
	ErrHitPointsOverrideTooSmall       = errors.New("effect: hit points override must be at least 1")
	ErrOverrideMaximumLessThanMinimum  = errors.New("effect: override maximum is less than the minimum")
	ErrAttackMultiplierPercentTooSmall = errors.New("effect: attack multiplier percent must be at least 1")
	ErrTollMultiplierPercentTooSmall   = errors.New("effect: toll multiplier percent must be at least 1")
	ErrCreatureElementsEmpty           = errors.New("effect: creature elements are empty")
	ErrCreatureElementUnknown          = errors.New("effect: unknown creature element")
	ErrCreatureElementsDuplicated      = errors.New("effect: duplicated creature element")
	ErrConstraintEmpty                 = errors.New("effect: constraint has no kind")
	ErrEffectEmpty                     = errors.New("effect: effect has no kind")
	ErrKindUnknown                     = errors.New("effect: unknown effect kind")
	ErrDrawCountTooSmall               = errors.New("effect: draw count must be at least 1")
	ErrMagicPerLapTooSmall             = errors.New("effect: magic per lap must be at least 1")
	ErrDiceCountTooSmall               = errors.New("effect: dice count must be at least 2")
	ErrDiceFaceTooSmall                = errors.New("effect: dice face must be at least 1")
	ErrEffectTurnsTooSmall             = errors.New("effect: effect turns must be at least 1")
	ErrDamageAmountTooSmall            = errors.New("effect: damage amount must be at least 1")
	ErrDestructionMagicAmountTooSmall  = errors.New("effect: destruction magic amount must be at least 1")
	ErrDestructionsEmpty               = errors.New("effect: destructions are empty")
	ErrDestructionUnknown              = errors.New("effect: unknown destruction")
	ErrDestructionsDuplicated          = errors.New("effect: duplicated destruction")
)
