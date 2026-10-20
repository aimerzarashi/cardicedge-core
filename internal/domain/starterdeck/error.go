package starterdeck

import "errors"

var (
	ErrInvalidState        = errors.New("starter deck: invalid state")
	ErrNameEmpty           = errors.New("starter deck: name is empty")
	ErrNameTooLong         = errors.New("starter deck: name is too long")
	ErrNameAlreadyInUse    = errors.New("starter deck: name already in use")
	ErrCardCountOutOfRange = errors.New("starter deck: card count is out of range")
	ErrTotalCountTooLarge  = errors.New("starter deck: total card count is too large")
	ErrNotCard             = errors.New("starter deck: not a card")
)
