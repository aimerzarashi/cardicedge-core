package element

import "errors"

var (
	ErrCreatureUnknown = errors.New("element: unknown creature element")
	ErrLandUnknown     = errors.New("element: unknown land element")
)
