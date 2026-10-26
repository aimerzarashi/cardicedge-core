package avatar

import "errors"

var (
	ErrInvalidState      = errors.New("avatar: invalid state")
	ErrAlreadyRegistered = errors.New("avatar: already registered")
	ErrNameEmpty         = errors.New("avatar: name is empty")
	ErrNameTooLong       = errors.New("avatar: name is too long")
	// 同じ持ち主のアバターで、正規化した名前が重なる
	ErrNameAlreadyInUse = errors.New("avatar: name already in use")
	// プレイヤーが持てるアバターの数の上限に達している
	ErrLimitReached = errors.New("avatar: limit reached")
)
