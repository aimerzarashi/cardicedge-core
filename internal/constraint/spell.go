package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
)

var _ spell.UpdateNameFunc = UpdateSpellName

func UpdateSpellName(ctx context.Context, id spell.ID, name card.Name) error {
	return updateCardName(ctx, id.Int64(), name)
}
