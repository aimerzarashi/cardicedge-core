package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

var _ collection.UpdateOwnerFunc = UpdateCollectionOwner

func UpdateCollectionOwner(ctx context.Context, id collection.ID, owner player.ID) error {
	return updateUniqueKey(ctx, id.Int64(), collection.OwnerKey(owner), collection.ErrAlreadyCreated)
}
