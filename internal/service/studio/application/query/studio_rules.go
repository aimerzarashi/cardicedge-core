package query

import (
	"context"
	"errors"
)

// studio_rules の行がまだ無い（起動時に作った直後で、投影が追いついていない）。ルールの値を黙って既定の値に戻さず、
// 失敗として返す（古い値や既定の値を、画面が入力の範囲として使わないため。待てば直る失敗なので、画面は読み直せばよい）
var ErrStudioRulesNotProjected = errors.New("query: studio rules not projected yet")

// presentation 層にドメインの型と read model の作りを見せないため、表示に要る値だけを持つ
type StudioRules struct {
	Board BoardRules
}

// ボードの幅・高さの範囲（幅・高さに共通。両端を含む）
type BoardRules struct {
	MinSize int
	MaxSize int
}

// 無ければ ErrStudioRulesNotProjected
type GetStudioRulesFunc func(ctx context.Context) (StudioRules, error)

type StudioRulesHandler struct {
	getStudioRules GetStudioRulesFunc
}

func NewStudioRulesHandler(getStudioRules GetStudioRulesFunc) *StudioRulesHandler {
	return &StudioRulesHandler{getStudioRules: getStudioRules}
}

func (q *StudioRulesHandler) Get(ctx context.Context) (StudioRules, error) {
	return q.getStudioRules(ctx)
}
