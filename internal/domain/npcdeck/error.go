package npcdeck

import "errors"

var (
	ErrInvalidState     = errors.New("npcdeck: invalid state")
	ErrNameAlreadyInUse = errors.New("npcdeck: name already in use")
	ErrStatusUnknown    = errors.New("npcdeck: unknown status")
	// 準備中の NPC のデッキを使用不可にしようとした
	ErrInvalidStatusTransition = errors.New("npcdeck: invalid status transition")
	// 準備中でない NPC のデッキの名前・枚数を変える、または削除しようとした
	ErrNotDraft           = errors.New("npcdeck: not draft")
	ErrTotalCountTooLarge = errors.New("npcdeck: total card count is too large")
	// 同じカードを、枚数を決めるカードと外すカードの両方に入れた
	ErrCardConflict = errors.New("npcdeck: card is both set and removed")
	// 公開するときの合計が、ちょうど gamerules.Deck.TotalCardCount でない
	ErrTotalMismatch = errors.New("npcdeck: total card count mismatch")
)
