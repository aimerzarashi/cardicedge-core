package card

import "errors"

var (
	ErrNameEmpty                 = errors.New("card: name is empty")
	ErrNameTooLong               = errors.New("card: name is too long")
	ErrNameAlreadyInUse          = errors.New("card: name already in use")
	ErrStatusUnknown             = errors.New("card: unknown status")
	ErrQuantityNotPositive       = errors.New("card: quantity must be positive")
	ErrDeckTotalCountNotPositive = errors.New("card: deck total count must be positive")
	// デッキに入れる枚数が、同じカードを入れられる上限（gamerules.Deck.MaxCardDuplicates）を超える
	ErrDeckQuantityOverMax = errors.New("card: deck quantity is over the max")
	ErrNotCard             = errors.New("card: not a card")
	// 準備中のカードを使用不可にしようとした
	ErrInvalidStatusTransition = errors.New("card: invalid status transition")
)
