package deck

import "errors"

var (
	ErrInvalidState = errors.New("deck: invalid state")
	// 作ってある状態への作成と、同じ配布の同じ番号のデッキがもうある場合の、どちらも同じ理由
	ErrAlreadyCreated = errors.New("deck: already created")
	ErrNameEmpty      = errors.New("deck: name is empty")
	ErrNameTooLong    = errors.New("deck: name is too long")
	// 同じ持ち主のデッキで、正規化した名前が重なる
	ErrNameAlreadyInUse = errors.New("deck: name already in use")
	// プレイヤーが持てるデッキの数の上限に達している
	ErrLimitReached = errors.New("deck: limit reached")
)
