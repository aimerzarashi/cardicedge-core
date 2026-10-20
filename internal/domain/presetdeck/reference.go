package presetdeck

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

// domain_reference_types の名前。プリセットデッキに入れたカードを、入れている間は消せないようにする
var cardReferenceType = domain.NewReferenceType("preset_deck.card")

func ReferenceTypes() []domain.ReferenceType {
	return []domain.ReferenceType{cardReferenceType}
}

func CardReferenceType() domain.ReferenceType {
	return cardReferenceType
}

// プリセットデッキが参照するカードを cards にそろえる（無いものは足し、外れたものは消す）。コマンドがイベントの差分を
// 知らなくても、変えた後の状態（PresetDeck.Cards）を渡すだけで済むよう、差分ではなく変えた後の一覧を受け取る
type UpdateCardReferencesFunc func(ctx context.Context, id ID, cards []CardRef) error
