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
