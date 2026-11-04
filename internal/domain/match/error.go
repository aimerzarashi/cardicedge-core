package match

import "errors"

var (
	ErrInvalidState   = errors.New("match: invalid state")
	ErrAlreadyCreated = errors.New("match: already created")
	// 写したボードに拠点が無い（公開したボードは拠点をちょうど 1 つ持つので、写し方の誤り）
	ErrBaseNotFound = errors.New("match: base not found")
)
