package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
)

var _ deck.UpdateStarterFunc = UpdateDeckStarter

func UpdateDeckStarter(ctx context.Context, id deck.ID, grant startergrant.ID, number startergrant.DeckNumber) error {
	return updateUniqueKey(ctx, id.Int64(), deck.StarterKey(grant, number), deck.ErrAlreadyCreated)
}
