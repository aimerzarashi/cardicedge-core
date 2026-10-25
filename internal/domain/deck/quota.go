package deck

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// domain_quota_types の名前。プレイヤー 1 人が持てるデッキの数
var PlayerQuotaType = domain.NewQuotaType("deck.per_player")

func QuotaTypes() []domain.QuotaType {
	return []domain.QuotaType{PlayerQuotaType}
}

// プレイヤーごとにデッキの数を数える
func PlayerQuota(owner player.ID) domain.Quota {
	return domain.NewQuota(PlayerQuotaType, owner.Int64())
}

// 上限に達していれば ErrLimitReached
type ReserveQuotaFunc func(ctx context.Context, quota domain.Quota, limit domain.QuotaLimit) error

// 確保したデッキの数を 1 つ戻す
type ReleaseQuotaFunc func(ctx context.Context, quota domain.Quota) error
