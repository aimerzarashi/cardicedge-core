package grantdeck

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

// domain_reference_types の名前。配布デッキに入れたカードを、入れている間は消せないようにする（プリセットデッキと同じ）
var cardReferenceType = domain.NewReferenceType("grant_deck.card")

func ReferenceTypes() []domain.ReferenceType {
	return []domain.ReferenceType{cardReferenceType}
}

func CardReferenceType() domain.ReferenceType {
	return cardReferenceType
}

// 配布デッキが参照するカードを cards にそろえる（presetdeck.UpdateCardReferencesFunc と同じ形）
type UpdateCardReferencesFunc func(ctx context.Context, id ID, cards []card.Ref) error
