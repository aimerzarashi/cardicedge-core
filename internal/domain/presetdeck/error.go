package presetdeck

import "errors"

var (
	ErrInvalidState     = errors.New("grant deck: invalid state")
	ErrNameEmpty        = errors.New("grant deck: name is empty")
	ErrNameTooLong      = errors.New("grant deck: name is too long")
	ErrNameAlreadyInUse = errors.New("grant deck: name already in use")
	// 枚数が負、または同じカードの枚数の上限（gamerules.Deck.MaxCardDuplicates）を超える
	ErrCardCountOutOfRange = errors.New("grant deck: card count is out of range")
	ErrTotalCountTooLarge  = errors.New("grant deck: total card count is too large")
	ErrNotCard             = errors.New("grant deck: not a card")
)
