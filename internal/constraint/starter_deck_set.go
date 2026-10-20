package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset"
)

var (
	_ starterdeckset.CreateFunc               = CreateStarterDeckSet
	_ starterdeckset.FindFunc                 = FindStarterDeckSet
	_ starterdeckset.UpdateCardReferencesFunc = UpdateStarterDeckSetCardReferences
)

func CreateStarterDeckSet(ctx context.Context, id starterdeckset.ID) error {
	return updateUniqueKey(ctx, id.Int64(), starterdeckset.SingletonKey(), starterdeckset.ErrAlreadyCreated)
}

func FindStarterDeckSet(ctx context.Context) (starterdeckset.ID, bool, error) {
	id, found, err := findUniqueKeyOwner(ctx, starterdeckset.SingletonKey())
	if err != nil || !found {
		return 0, false, err
	}
	starterDeckSetID, err := starterdeckset.NewID(id)
	return starterDeckSetID, err == nil, err
}

func UpdateStarterDeckSetCardReferences(ctx context.Context, id starterdeckset.ID, cards []presetdeck.CardRef) error {
	ids := make([]int64, 0, len(cards))
	for _, card := range cards {
		ids = append(ids, card.ID())
	}
	return updateReferences(ctx, starterdeckset.CardReferenceType(), id.Int64(), ids)
}
