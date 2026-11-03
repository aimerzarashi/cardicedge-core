package starterset

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
)

// 同時に公開中にできるスターターセットの数。プレイヤーがセットを選べる形にするまで値を変える場面が無いため、
// game_rules には置かずここに固定する（上限の値を変えたくなったら game_rules の項目にする）
const MaxPublished = 1

// domain_quota_types の名前。公開中のスターターセットの数
var PublishedQuotaType = domain.NewQuotaType("starter_set.published")

func QuotaTypes() []domain.QuotaType {
	return []domain.QuotaType{PublishedQuotaType}
}

// ゲーム全体で 1 つ数える。持ち主は、ゲーム全体のルールの集約の内部 ID（数える単位の持ち主として使う）
func PublishedQuota(gameRulesID gamerules.ID) domain.Quota {
	return domain.NewQuota(PublishedQuotaType, gameRulesID.Int64())
}

// 上限に達していれば ErrLimitReached
type ReserveQuotaFunc func(ctx context.Context, quota domain.Quota, limit domain.QuotaLimit) error

// 確保した公開中の数を 1 つ戻す
type ReleaseQuotaFunc func(ctx context.Context, quota domain.Quota) error
