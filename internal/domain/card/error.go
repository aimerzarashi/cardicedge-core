package card

import "errors"

var (
	ErrNameEmpty        = errors.New("card: name is empty")
	ErrNameTooLong      = errors.New("card: name is too long")
	ErrNameAlreadyInUse = errors.New("card: name already in use")
)
