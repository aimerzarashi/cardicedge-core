package operator

import "errors"

var (
	ErrInvalidState      = errors.New("operator: invalid state")
	ErrAlreadyRegistered = errors.New("operator: email already registered")
	ErrAlreadyLinked     = errors.New("operator: already linked")
)
