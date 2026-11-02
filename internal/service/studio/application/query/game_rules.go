package query

import (
	"context"
	"errors"
)

// game_rules の行がまだ無い（起動時に作った直後で、投影が追いついていない）。ルールの値を黙って既定の値に戻さず、
// 失敗として返す（画面が既定の値を、運営が決めた値のように見せないため。待てば直る失敗なので、画面は読み直せばよい）
var ErrGameRulesNotProjected = errors.New("query: game rules not projected yet")

// presentation 層にドメインの型と read model の作りを見せないため、表示に要る値だけを持つ。
// 値の変更（core#23）や、ほかの項目を見せる要件ができたら、ここに足す
type GameRules struct {
	Match MatchRules
}

// 対戦の決まり
type MatchRules struct {
	MinPlayers   int
	MaxPlayers   int
	MaxHandCards int
}

// 無ければ ErrGameRulesNotProjected
type GetGameRulesFunc func(ctx context.Context) (GameRules, error)

type GameRulesHandler struct {
	getGameRules GetGameRulesFunc
}

func NewGameRulesHandler(getGameRules GetGameRulesFunc) *GameRulesHandler {
	return &GameRulesHandler{getGameRules: getGameRules}
}

func (q *GameRulesHandler) Get(ctx context.Context) (GameRules, error) {
	return q.getGameRules(ctx)
}
