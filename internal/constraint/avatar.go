package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

var (
	_ avatar.UpdateNameFunc   = UpdateAvatarName
	_ avatar.ReserveQuotaFunc = ReserveAvatarQuota
)

func UpdateAvatarName(ctx context.Context, id avatar.ID, owner player.ID, name avatar.Name) error {
	return updateUniqueKey(ctx, id.Int64(), avatar.NameKey(owner, name), avatar.ErrNameAlreadyInUse)
}

func ReserveAvatarQuota(ctx context.Context, quota domain.Quota, limit domain.QuotaLimit) error {
	return reserveQuota(ctx, quota, limit, avatar.ErrLimitReached)
}
