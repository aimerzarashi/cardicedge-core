package startergrant

import "errors"

var (
	ErrInvalidState = errors.New("starter grant: invalid state")
	// 始めてある状態への開始と、そのプレイヤーにほかの配布が始まっている場合の、どちらも同じ理由（もう始めてある）
	ErrAlreadyStarted    = errors.New("starter grant: already started")
	ErrInvalidDeckCount  = errors.New("starter grant: deck count must be two")
	ErrInvalidDeckNumber = errors.New("starter grant: invalid deck number")
	ErrCountNotPositive  = errors.New("starter grant: count must be positive")
	ErrDeckNameEmpty     = errors.New("starter grant: deck name is empty")
	ErrDeckNameTooLong   = errors.New("starter grant: deck name is too long")
)
