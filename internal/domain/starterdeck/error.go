package starterdeck

import "errors"

var (
	ErrInvalidState     = errors.New("starter deck: invalid state")
	ErrNameAlreadyInUse = errors.New("starter deck: name already in use")
	ErrStatusUnknown    = errors.New("starter deck: unknown status")
	// 準備中のスターターデッキを使用不可にしようとした
	ErrInvalidStatusTransition = errors.New("starter deck: invalid status transition")
	// 準備中でないスターターデッキの名前・枚数を変える、または削除しようとした
	ErrNotDraft = errors.New("starter deck: not draft")
	// 枚数が負、または同じカードの枚数の上限（gamerules.Deck.MaxCardDuplicates）を超える
	ErrCardCountOutOfRange = errors.New("starter deck: card count is out of range")
	ErrTotalCountTooLarge  = errors.New("starter deck: total card count is too large")
	// 公開するときの合計が、ちょうど gamerules.Deck.TotalCardCount でない
	ErrTotalMismatch = errors.New("starter deck: total card count mismatch")
	// 公開中の数の上限（gamerules.StarterDeck.MaxPublished）に達している
	ErrLimitReached = errors.New("starter deck: limit reached")
)
