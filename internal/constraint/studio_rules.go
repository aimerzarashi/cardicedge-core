package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
)

var (
	_ studiorules.CreateFunc = CreateStudioRules
	_ studiorules.FindFunc   = FindStudioRules
)

func CreateStudioRules(ctx context.Context, id studiorules.ID) error {
	return updateUniqueKey(ctx, id.Int64(), studiorules.SingletonKey(), studiorules.ErrAlreadyCreated)
}

func FindStudioRules(ctx context.Context) (studiorules.ID, bool, error) {
	id, found, err := findUniqueKeyOwner(ctx, studiorules.SingletonKey())
	if err != nil || !found {
		return 0, false, err
	}
	studioRulesID, err := studiorules.NewID(id)
	return studioRulesID, err == nil, err
}
