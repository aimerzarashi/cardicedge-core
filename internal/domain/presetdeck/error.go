package presetdeck

import "errors"

var (
	ErrInvalidState     = errors.New("starter deck: invalid state")
	ErrNameEmpty        = errors.New("starter deck: name is empty")
	ErrNameTooLong      = errors.New("starter deck: name is too long")
	ErrNameAlreadyInUse = errors.New("starter deck: name already in use")
	// 枚数が負、または同じカードの枚数の上限（gamerules.Deck.MaxCardDuplicates）を超える
	ErrCardCountOutOfRange = errors.New("starter deck: card count is out of range")
	ErrTotalCountTooLarge  = errors.New("starter deck: total card count is too large")
	ErrNotCard             = errors.New("starter deck: not a card")
)
