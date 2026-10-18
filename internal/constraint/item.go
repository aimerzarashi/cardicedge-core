package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
)

var _ item.UpdateNameFunc = UpdateItemName

func UpdateItemName(ctx context.Context, id item.ID, name card.Name) error {
	return updateCardName(ctx, id.Int64(), name)
}
