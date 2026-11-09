package item

import "errors"

var (
	ErrInvalidState    = errors.New("item: invalid state")
	ErrCategoryInvalid = errors.New("item: category invalid")
)
