package studiorules

import "errors"

var (
	ErrInvalidState = errors.New("studio rules: invalid state")
	// 作ってある状態への作成と、全体で 1 つの key をほかが先に確保していた場合の、どちらも同じ理由
	ErrAlreadyCreated = errors.New("studio rules: already created")
)
