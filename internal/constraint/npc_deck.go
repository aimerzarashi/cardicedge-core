package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

var (
	_ npcdeck.UpdateNameFunc           = UpdateNPCDeckName
	_ npcdeck.UpdateCardReferencesFunc = UpdateNPCDeckCardReferences
)

func UpdateNPCDeckName(ctx context.Context, id npcdeck.ID, name presetdeck.Name) error {
	return updateUniqueKey(ctx, id.Int64(), npcdeck.NameKey(name), npcdeck.ErrNameAlreadyInUse)
}

func UpdateNPCDeckCardReferences(ctx context.Context, id npcdeck.ID, cards []presetdeck.CardRef) error {
	ids := make([]int64, 0, len(cards))
	for _, card := range cards {
		ids = append(ids, card.ID())
	}
	return updateReferences(ctx, npcdeck.CardReferenceType(), id.Int64(), ids)
}
