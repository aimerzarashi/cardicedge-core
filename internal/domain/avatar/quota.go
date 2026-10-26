package avatar

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// domain_quota_types の名前。プレイヤー 1 人が持てるアバターの数
var PlayerQuotaType = domain.NewQuotaType("avatar.per_player")

func QuotaTypes() []domain.QuotaType {
	return []domain.QuotaType{PlayerQuotaType}
}

// プレイヤーごとにアバターの数を数える
func PlayerQuota(playerID player.ID) domain.Quota {
	return domain.NewQuota(PlayerQuotaType, playerID.Int64())
}

// 上限に達していれば ErrLimitReached
type ReserveQuotaFunc func(ctx context.Context, quota domain.Quota, limit domain.QuotaLimit) error
