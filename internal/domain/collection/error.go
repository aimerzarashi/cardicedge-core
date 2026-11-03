package collection

import "errors"

var (
	ErrInvalidState = errors.New("collection: invalid state")
	// 作ってある状態への作成と、そのプレイヤーの所持品がもうある場合の、どちらも同じ理由
	ErrAlreadyCreated = errors.New("collection: already created")
)
