package model

import "github.com/google/uuid"

// read model から読んだカードの公開 ID と枚数。デッキ（プリセットデッキ・配布デッキ）の cards の resolver が、
// カード名を引いて並べるまで持つ（スキーマの型ではない）
type DeckCardCount struct {
	CardID uuid.UUID
	Count  int
}
