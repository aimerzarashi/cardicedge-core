package gamerules

import "errors"

var (
	ErrInvalidState = errors.New("game rules: invalid state")
	// 作ってある状態への作成と、ゲーム全体で 1 つの key をほかが先に確保していた場合の、どちらも同じ理由
	ErrAlreadyCreated = errors.New("game rules: already created")
)
