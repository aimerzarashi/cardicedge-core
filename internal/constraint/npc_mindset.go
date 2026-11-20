package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
)

var _ npcmindset.UpdateNameFunc = UpdateNPCMindsetName

func UpdateNPCMindsetName(ctx context.Context, id npcmindset.ID, name npcmindset.Name) error {
	return updateUniqueKey(ctx, id.Int64(), npcmindset.NameKey(name), npcmindset.ErrNameAlreadyInUse)
}
