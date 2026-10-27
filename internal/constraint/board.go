package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
)

var _ board.UpdateNameFunc = UpdateBoardName

func UpdateBoardName(ctx context.Context, id board.ID, name board.Name) error {
	return updateUniqueKey(ctx, id.Int64(), board.NameKey(name), board.ErrNameAlreadyInUse)
}
