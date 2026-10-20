package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
)

var (
	_ starterdeck.UpdateNameFunc           = UpdateStarterDeckName
	_ starterdeck.UpdateCardReferencesFunc = UpdateStarterDeckCardReferences
)

func UpdateStarterDeckName(ctx context.Context, id starterdeck.ID, name starterdeck.Name) error {
	return updateUniqueKey(ctx, id.Int64(), starterdeck.NameKey(name), starterdeck.ErrNameAlreadyInUse)
}

func UpdateStarterDeckCardReferences(ctx context.Context, id starterdeck.ID, cards []starterdeck.CardRef) error {
	ids := make([]int64, 0, len(cards))
	for _, card := range cards {
		ids = append(ids, card.ID())
	}
	return updateReferences(ctx, starterdeck.CardReferenceType(), id.Int64(), ids)
}
