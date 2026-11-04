package query_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules/gamerulestest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

func TestGameRulesHandler_ReturnsMatchRulesFromPublishedRules(t *testing.T) {
	rules := gamerules.Defaults()
	rules.Match = gamerulestest.Match(t, 3, 5, 7)
	reader := func(context.Context) (gamerules.Rules, bool, error) { return rules, true, nil }

	got, err := query.NewGameRulesHandler(reader).Get(t.Context())

	require.NoError(t, err)
	match := query.MatchRules{MinPlayers: 3, MaxPlayers: 5, MaxHandCards: 7}
	require.Equal(t, query.GameRules{Match: match}, got)
}

// 写されていないとき、既定の値に戻さずエラーを返す
func TestGameRulesHandler_FailsWhenNotPublished(t *testing.T) {
	reader := func(context.Context) (gamerules.Rules, bool, error) { return gamerules.Rules{}, false, nil }

	_, err := query.NewGameRulesHandler(reader).Get(t.Context())

	require.ErrorIs(t, err, query.ErrGameRulesNotPublished)
}
