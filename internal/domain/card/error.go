package card

import "errors"

var (
	ErrNameEmpty        = errors.New("card: name is empty")
	ErrNameTooLong      = errors.New("card: name is too long")
	ErrNameAlreadyInUse = errors.New("card: name already in use")
	ErrStatusUnknown    = errors.New("card: unknown status")
	// 準備中のカードを使用不可にしようとした
	ErrInvalidStatusTransition = errors.New("card: invalid status transition")
)
