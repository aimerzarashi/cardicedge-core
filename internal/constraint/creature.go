package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
)

var _ creature.UpdateNameFunc = UpdateCreatureName

func UpdateCreatureName(ctx context.Context, id creature.ID, name card.Name) error {
	return updateCardName(ctx, id.Int64(), name)
}
