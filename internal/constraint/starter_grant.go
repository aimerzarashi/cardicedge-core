package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
)

var _ startergrant.UpdatePlayerFunc = UpdateStarterGrantPlayer

func UpdateStarterGrantPlayer(ctx context.Context, id startergrant.ID, playerID player.ID) error {
	return updateUniqueKey(ctx, id.Int64(), startergrant.PlayerKey(playerID), startergrant.ErrAlreadyStarted)
}

// プレイヤーの配布（プレイヤーに 1 つ）を、一意な key から引く。無ければ found が false
func FindStarterGrantByPlayer(ctx context.Context, playerID player.ID) (startergrant.ID, bool, error) {
	id, found, err := findUniqueKeyOwner(ctx, startergrant.PlayerKey(playerID))
	if err != nil || !found {
		return 0, false, err
	}
	grantID, err := startergrant.NewID(id)
	return grantID, err == nil, err
}
