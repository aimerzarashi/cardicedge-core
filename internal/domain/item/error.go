package item

import "errors"

var (
	ErrInvalidState                    = errors.New("item: invalid state")
	ErrCategoryInvalid                 = errors.New("item: category invalid")
	ErrAttackOverrideNegative          = errors.New("item: attack override must not be negative")
	ErrAttackMultiplierPercentTooSmall = errors.New("item: attack multiplier percent must be at least 1")
	ErrElementsEmpty                   = errors.New("item: elements are empty")
	ErrElementUnknown                  = errors.New("item: unknown element")
	ErrElementsDuplicated              = errors.New("item: duplicated element")
	ErrConstraintEmpty                 = errors.New("item: constraint has no kind")
	ErrEffectEmpty                     = errors.New("item: effect has no kind")
	ErrEffectKindUnknown               = errors.New("item: unknown effect kind")
	// 分類で持てない効果の種類（効果を変えるとき・分類を変えるとき）
	ErrEffectNotAllowed = errors.New("item: effect not allowed in the category")
)
