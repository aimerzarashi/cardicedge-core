package starterdeckset

import "errors"

var (
	ErrInvalidState = errors.New("starter deck set: invalid state")
	// スターターデッキの数が RequiredDeckCount でない
	ErrDeckCountMismatch = errors.New("starter deck set: deck count mismatch")
	// 同じプリセットデッキから 2 つ写そうとした
	ErrDuplicatedDeck = errors.New("starter deck set: duplicated deck")
	// 写す元のプリセットデッキの合計が、ちょうど presetdeck.MaxTotalCount でない
	ErrDeckNotSelectable = errors.New("starter deck set: deck is not selectable")
	// ほかの人が先に初めての設定をした（一意な key が確保されていた）
	ErrAlreadyCreated = errors.New("starter deck set: already created")
)
