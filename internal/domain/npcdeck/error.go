package npcdeck

import "errors"

var (
	ErrInvalidState     = errors.New("npcdeck: invalid state")
	ErrNameAlreadyInUse = errors.New("npcdeck: name already in use")
	ErrStatusUnknown    = errors.New("npcdeck: unknown status")
	// 準備中の NPC のデッキを使用不可にしようとした
	ErrInvalidStatusTransition = errors.New("npcdeck: invalid status transition")
	// 準備中でない NPC のデッキの名前・枚数を変える、または削除しようとした
	ErrNotDraft = errors.New("npcdeck: not draft")
	// 枚数が負、または同じカードの枚数の上限（gamerules.Deck.MaxCardDuplicates）を超える
	ErrCardCountOutOfRange = errors.New("npcdeck: card count is out of range")
	ErrTotalCountTooLarge  = errors.New("npcdeck: total card count is too large")
	// 公開するときの合計が、ちょうど gamerules.Deck.TotalCardCount でない
	ErrTotalMismatch = errors.New("npcdeck: total card count mismatch")
)
