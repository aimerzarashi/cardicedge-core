package creature

import "errors"

var ErrInvalidState = errors.New("creature: invalid state")

var (
	ErrElementsEmpty        = errors.New("creature: elements are empty")
	ErrElementUnknown       = errors.New("creature: unknown element")
	ErrElementsDuplicated   = errors.New("creature: duplicated element")
	ErrElementsNeutralMixed = errors.New("creature: neutral is mixed with other elements")
	ErrAttackNegative       = errors.New("creature: attack is negative")
	ErrHitPointsTooSmall    = errors.New("creature: hit points are less than 1")

	ErrItemRestrictionUnknown     = errors.New("creature: unknown item restriction")
	ErrItemRestrictionsDuplicated = errors.New("creature: duplicated item restriction")

	ErrPlacementRestrictionUnknown     = errors.New("creature: unknown placement restriction")
	ErrPlacementRestrictionsDuplicated = errors.New("creature: duplicated placement restriction")

	ErrAbilityEmpty                            = errors.New("creature: ability has no kind")
	ErrAbilitiesDuplicated                     = errors.New("creature: duplicated ability")
	ErrAbilitiesConflicted                     = errors.New("creature: first strike and last strike cannot be combined")
	ErrBattleRandomStatsMinimumTooSmall        = errors.New("creature: battle random stats minimum is less than 1")
	ErrBattleRandomStatsMaximumLessThanMinimum = errors.New("creature: battle random stats maximum is less than minimum")
	ErrTollMultiplierPercentTooSmall           = errors.New("creature: toll multiplier percent is less than 1")
)
