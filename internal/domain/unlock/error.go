package unlock

import "errors"

var (
	ErrInvalidState = errors.New("unlock: invalid state")
	// 作ってある状態への作成と、そのプレイヤーの解放がもうある場合の、どちらも同じ理由
	ErrAlreadyCreated = errors.New("unlock: already created")
)
