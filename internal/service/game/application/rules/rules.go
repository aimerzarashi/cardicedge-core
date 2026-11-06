// 全体のルール（運営が決める、プレイヤーによらない値）を読む。写した game_rules を読む関数は internal/service/game が渡す
package rules

import (
	"context"
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
)

// 上限の種別ごとの、今の上限の値
type GetQuotaLimitFunc func(ctx context.Context, quotaType domain.QuotaType) (domain.QuotaLimit, error)

// 写した game_rules を読む。まだ写されていなければ found は false（マスタと同じ。エラーにするかは呼ぶ側が決める）
type GetRulesFunc func(ctx context.Context) (rules gamerules.Rules, found bool, err error)

// まだ game_rules が写されていない。配布のポリシーでは、写されれば直る失敗になる（マスタの ErrMasterNotPublished と同じ扱い）
var ErrNotPublished = errors.New("rules: game rules not published")

// 写した game_rules から上限を読む。写されていなければ ErrNotPublished で、ドメインの既定の値に黙って戻さない
// （写し忘れに気づけず、studio と違う値で動くため）。対応に無い種別もエラーにする（足し忘れが上限なしの動きになるため）。
// コマンドは読む関数だけを知るので、値の持ち方を変えてもコマンドは変わらない
func FromPublished(get GetRulesFunc) GetQuotaLimitFunc {
	return func(ctx context.Context, quotaType domain.QuotaType) (domain.QuotaLimit, error) {
		rules, found, err := get(ctx)
		if err != nil {
			return 0, err
		}
		if !found {
			return 0, ErrNotPublished
		}
		switch quotaType {
		case deck.PlayerQuotaType:
			return domain.NewQuotaLimit(rules.Player.MaxDecks.Int())
		case avatar.PlayerQuotaType:
			return domain.NewQuotaLimit(rules.Player.MaxAvatars.Int())
		default:
			return 0, fmt.Errorf("quota limit is not defined: %s", quotaType)
		}
	}
}
