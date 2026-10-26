package starterdeck

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
)

// domain_quota_types の名前。公開中のスターターデッキの数
var PublishedQuotaType = domain.NewQuotaType("starter_deck.published")

func QuotaTypes() []domain.QuotaType {
	return []domain.QuotaType{PublishedQuotaType}
}

// ゲーム全体で 1 つ数える。持ち主は、ゲーム全体のルールの集約（上限の値を持つ集約）の内部 ID
func PublishedQuota(gameRulesID gamerules.ID) domain.Quota {
	return domain.NewQuota(PublishedQuotaType, gameRulesID.Int64())
}

// 上限に達していれば ErrLimitReached
type ReserveQuotaFunc func(ctx context.Context, quota domain.Quota, limit domain.QuotaLimit) error

// 確保した公開中の数を 1 つ戻す
type ReleaseQuotaFunc func(ctx context.Context, quota domain.Quota) error
