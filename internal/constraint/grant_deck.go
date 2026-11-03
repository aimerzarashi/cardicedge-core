package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

var (
	_ grantdeck.UpdateNameFunc           = UpdateGrantDeckName
	_ grantdeck.UpdateCardReferencesFunc = UpdateGrantDeckCardReferences
)

func UpdateGrantDeckName(ctx context.Context, id grantdeck.ID, name presetdeck.Name) error {
	return updateUniqueKey(ctx, id.Int64(), grantdeck.NameKey(name), grantdeck.ErrNameAlreadyInUse)
}

func UpdateGrantDeckCardReferences(ctx context.Context, id grantdeck.ID, cards []presetdeck.CardRef) error {
	ids := make([]int64, 0, len(cards))
	for _, card := range cards {
		ids = append(ids, card.ID())
	}
	return updateReferences(ctx, grantdeck.CardReferenceType(), id.Int64(), ids)
}
