package query

import (
	"context"
	"errors"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
)

// まだ game_rules が写されていない。配布のポリシーでは、写されれば直る失敗になる。ルールの値を黙って既定の値に戻さない
// （写し忘れに気づけず、画面が studio と違う値を見せるため）
var ErrGameRulesNotPublished = errors.New("query: game rules not published")

// presentation 層にドメインの型を見せないため、表示に要る値だけを持つ
type GameRules struct {
	Match MatchRules
}

// 対戦の決まり
type MatchRules struct {
	MinPlayers   int
	MaxPlayers   int
	MaxHandCards int
}

// 写した game_rules を読む。まだ写されていなければ found は false
type GetGameRulesFunc func(ctx context.Context) (rules gamerules.Rules, found bool, err error)

type GameRulesHandler struct {
	getGameRules GetGameRulesFunc
}

func NewGameRulesHandler(getGameRules GetGameRulesFunc) *GameRulesHandler {
	return &GameRulesHandler{getGameRules: getGameRules}
}

func (q *GameRulesHandler) Get(ctx context.Context) (GameRules, error) {
	rules, found, err := q.getGameRules(ctx)
	if err != nil {
		return GameRules{}, err
	}
	if !found {
		return GameRules{}, ErrGameRulesNotPublished
	}
	match := rules.Match
	matchRules := MatchRules{MinPlayers: match.MinPlayers, MaxPlayers: match.MaxPlayers, MaxHandCards: match.MaxHandCards}
	return GameRules{Match: matchRules}, nil
}
