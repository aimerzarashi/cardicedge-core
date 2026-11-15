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

	ErrEffectNotAllowed  = errors.New("creature: effect not allowed for creatures")
	ErrEffectsConflicted = errors.New("creature: strike first and strike last cannot be combined")
)
