package starterdeckset

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

// domain_reference_types の名前。スターターデッキに写したカードを、写している間は消せないようにする（プリセットデッキと同じ）
var cardReferenceType = domain.NewReferenceType("starter_deck_set.card")

func ReferenceTypes() []domain.ReferenceType {
	return []domain.ReferenceType{cardReferenceType}
}

func CardReferenceType() domain.ReferenceType {
	return cardReferenceType
}

// スターターデッキセットが参照するカードを cards にそろえる（プリセットデッキの UpdateCardReferencesFunc と同じ形）
type UpdateCardReferencesFunc func(ctx context.Context, id ID, cards []presetdeck.CardRef) error
