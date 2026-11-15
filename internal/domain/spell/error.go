package spell

import "errors"

var (
	ErrInvalidState = errors.New("spell: invalid state")
	// 効果の無いスペルは公開できない
	ErrNoEffect = errors.New("spell: no effect")
)
