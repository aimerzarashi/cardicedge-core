package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/unlock"
)

var _ unlock.UpdatePlayerFunc = UpdateUnlockPlayer

func UpdateUnlockPlayer(ctx context.Context, id unlock.ID, playerID player.ID) error {
	return updateUniqueKey(ctx, id.Int64(), unlock.PlayerKey(playerID), unlock.ErrAlreadyCreated)
}
