package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

var (
	_ presetdeck.UpdateNameFunc           = UpdatePresetDeckName
	_ presetdeck.UpdateCardReferencesFunc = UpdatePresetDeckCardReferences
)

func UpdatePresetDeckName(ctx context.Context, id presetdeck.ID, name presetdeck.Name) error {
	return updateUniqueKey(ctx, id.Int64(), presetdeck.NameKey(name), presetdeck.ErrNameAlreadyInUse)
}

func UpdatePresetDeckCardReferences(ctx context.Context, id presetdeck.ID, cards []presetdeck.CardRef) error {
	ids := make([]int64, 0, len(cards))
	for _, card := range cards {
		ids = append(ids, card.ID())
	}
	return updateReferences(ctx, presetdeck.CardReferenceType(), id.Int64(), ids)
}
