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

// プレイヤーの所持品（プレイヤーに 1 つ）を、一意な key から引く。無ければ found が false
func FindCollectionByPlayer(ctx context.Context, playerID player.ID) (collection.ID, bool, error) {
	id, found, err := findUniqueKeyOwner(ctx, collection.PlayerKey(playerID))
	if err != nil || !found {
		return 0, false, err
	}
	collectionID, err := collection.NewID(id)
	return collectionID, err == nil, err
}
