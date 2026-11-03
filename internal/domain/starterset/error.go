package starterset

import "errors"

var (
	ErrInvalidState     = errors.New("starter set: invalid state")
	ErrNameEmpty        = errors.New("starter set: name is empty")
	ErrNameTooLong      = errors.New("starter set: name is too long")
	ErrNameAlreadyInUse = errors.New("starter set: name already in use")
	ErrStatusUnknown    = errors.New("starter set: unknown status")
	// 準備中のスターターセットを使用不可にしようとした
	ErrInvalidStatusTransition = errors.New("starter set: invalid status transition")
	// 準備中でないスターターセットの名前・配るデッキ・解放するボードを変える、または削除しようとした
	ErrNotDraft = errors.New("starter set: not draft")
	// 配るデッキが 1 つも無いまま公開しようとした（配布は 0 個のデッキでは始められない。startergrant.ErrInvalidDeckCount）
	ErrNoGrantDecks = errors.New("starter set: no grant decks")
	// 公開中の数の上限（MaxPublished）に達している
	ErrLimitReached = errors.New("starter set: limit reached")
)
