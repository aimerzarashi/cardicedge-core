package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
)

var (
	_ gamerules.CreateFunc = CreateGameRules
	_ gamerules.FindFunc   = FindGameRules
)

func CreateGameRules(ctx context.Context, id gamerules.ID) error {
	return updateUniqueKey(ctx, id.Int64(), gamerules.SingletonKey(), gamerules.ErrAlreadyCreated)
}

func FindGameRules(ctx context.Context) (gamerules.ID, bool, error) {
	id, found, err := findUniqueKeyOwner(ctx, gamerules.SingletonKey())
	if err != nil || !found {
		return 0, false, err
	}
	gameRulesID, err := gamerules.NewID(id)
	return gameRulesID, err == nil, err
}
