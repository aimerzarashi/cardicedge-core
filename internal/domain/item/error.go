package item

import "errors"

var (
	ErrInvalidState = errors.New("item: invalid state")
	// 分類で持てない効果の種類（効果を変えるとき・分類を変えるとき）
	ErrEffectNotAllowed = errors.New("item: effect not allowed in the category")
)
