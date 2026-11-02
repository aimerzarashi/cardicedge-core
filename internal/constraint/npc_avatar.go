package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
)

var (
	_ npcavatar.UpdateNameFunc           = UpdateNPCAvatarName
	_ npcavatar.UpdateDeckReferencesFunc = UpdateNPCAvatarDeckReferences
)

func UpdateNPCAvatarName(ctx context.Context, id npcavatar.ID, name npcavatar.Name) error {
	return updateUniqueKey(ctx, id.Int64(), npcavatar.NameKey(name), npcavatar.ErrNameAlreadyInUse)
}

func UpdateNPCAvatarDeckReferences(ctx context.Context, id npcavatar.ID, decks []npcdeck.ID) error {
	ids := make([]int64, 0, len(decks))
	for _, deck := range decks {
		ids = append(ids, deck.Int64())
	}
	return updateReferences(ctx, npcavatar.DeckReferenceType(), id.Int64(), ids)
}
