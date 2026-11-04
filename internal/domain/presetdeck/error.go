package presetdeck

import "errors"

var (
	ErrInvalidState       = errors.New("grant deck: invalid state")
	ErrNameEmpty          = errors.New("grant deck: name is empty")
	ErrNameTooLong        = errors.New("grant deck: name is too long")
	ErrNameAlreadyInUse   = errors.New("grant deck: name already in use")
	ErrTotalCountTooLarge = errors.New("preset deck: total card count is too large")
	// 同じカードを、枚数を決めるカードと外すカードの両方に入れた
	ErrCardConflict        = errors.New("preset deck: card is both set and removed")
	ErrRevisionNotPositive = errors.New("preset deck: revision must be positive")
)
