// 全体のルール（運営が決める、プレイヤーによらない値）を読む。読み出しの関数は internal/service/game が渡す
package rules

import (
	"context"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

// 上限の種別ごとの、今の上限の値
type GetQuotaLimitFunc func(ctx context.Context, quotaType domain.QuotaType) (domain.QuotaLimit, error)

// 既定の値の対応から作る。core#23 で、運営が調整した値を表から読む実装に差し替える。それまでは、ドメインが宣言した既定の値を返す
// （コマンドは読む関数だけを知るので、差し替えてもコマンドは変わらない）。対応に無い種別はエラーにする
// （既定を黙って補うと、足し忘れが上限なしの動きになるため）
func Defaults(limits map[domain.QuotaType]domain.QuotaLimit) GetQuotaLimitFunc {
	return func(_ context.Context, quotaType domain.QuotaType) (domain.QuotaLimit, error) {
		limit, ok := limits[quotaType]
		if !ok {
			return 0, fmt.Errorf("quota limit is not defined: %s", quotaType)
		}
		return limit, nil
	}
}
