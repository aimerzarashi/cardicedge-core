package player

import "errors"

var (
	ErrInvalidState = errors.New("player: invalid state")
	// 登録済みの状態への登録と、その (iss, sub) をほかのプレイヤーが使っている場合の、どちらも同じ理由（その人はもうプレイヤー）
	ErrAlreadyRegistered = errors.New("player: already registered")
)
