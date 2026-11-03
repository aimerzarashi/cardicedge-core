package grantdeck

import "errors"

var (
	ErrInvalidState     = errors.New("grant deck: invalid state")
	ErrNameAlreadyInUse = errors.New("grant deck: name already in use")
	ErrStatusUnknown    = errors.New("grant deck: unknown status")
	// 準備中の配布デッキを使用不可にしようとした
	ErrInvalidStatusTransition = errors.New("grant deck: invalid status transition")
	// 準備中でない配布デッキの名前・枚数を変える、または削除しようとした
	ErrNotDraft = errors.New("grant deck: not draft")
	// 枚数が負、または同じカードの枚数の上限（gamerules.Deck.MaxCardDuplicates）を超える
	ErrCardCountOutOfRange = errors.New("grant deck: card count is out of range")
	ErrTotalCountTooLarge  = errors.New("grant deck: total card count is too large")
	// 公開するときの合計が、ちょうど gamerules.Deck.TotalCardCount でない
	ErrTotalMismatch = errors.New("grant deck: total card count mismatch")
)
