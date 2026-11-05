package collection

import "errors"

var (
	ErrInvalidState = errors.New("collection: invalid state")
	// 作ってある状態への作成と、そのプレイヤーの所持品がもうある場合の、どちらも同じ理由
	ErrAlreadyCreated = errors.New("collection: already created")
	// 同じ出どころ（同じ対戦の戦利品）の分はもう足してある
	ErrAlreadyAcquired = errors.New("collection: already acquired")
	// 足すカードが無い、または出どころが無い
	ErrInvalidAcquisition = errors.New("collection: invalid acquisition")
)
