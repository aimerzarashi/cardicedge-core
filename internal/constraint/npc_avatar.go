package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
)

var _ npcavatar.UpdateNameFunc = UpdateNPCAvatarName

func UpdateNPCAvatarName(ctx context.Context, id npcavatar.ID, name npcavatar.Name) error {
	return updateUniqueKey(ctx, id.Int64(), npcavatar.NameKey(name), npcavatar.ErrNameAlreadyInUse)
}
