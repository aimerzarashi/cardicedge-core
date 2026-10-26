package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

var _ collection.UpdatePlayerFunc = UpdateCollectionPlayer

func UpdateCollectionPlayer(ctx context.Context, id collection.ID, playerID player.ID) error {
	return updateUniqueKey(ctx, id.Int64(), collection.PlayerKey(playerID), collection.ErrAlreadyCreated)
}
