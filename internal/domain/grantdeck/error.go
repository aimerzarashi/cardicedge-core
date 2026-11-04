package grantdeck

import "errors"

var (
	ErrInvalidState     = errors.New("grant deck: invalid state")
	ErrNameAlreadyInUse = errors.New("grant deck: name already in use")
	ErrStatusUnknown    = errors.New("grant deck: unknown status")
	// 準備中の配布デッキを使用不可にしようとした
	ErrInvalidStatusTransition = errors.New("grant deck: invalid status transition")
	// 準備中でない配布デッキの名前・枚数を変える、または削除しようとした
	ErrNotDraft           = errors.New("grant deck: not draft")
	ErrTotalCountTooLarge = errors.New("grant deck: total card count is too large")
	// 同じカードを、枚数を決めるカードと外すカードの両方に入れた
	ErrCardConflict = errors.New("grant deck: card is both set and removed")
	// 公開するときの合計が、ちょうど gamerules.Deck.TotalCardCount でない
	ErrTotalMismatch = errors.New("grant deck: total card count mismatch")
)
