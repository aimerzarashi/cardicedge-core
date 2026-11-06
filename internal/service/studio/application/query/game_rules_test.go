package query_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
)

func TestGameRulesHandler_ReturnsRulesFromReader(t *testing.T) {
	match := query.MatchRules{MinPlayers: 2, MaxPlayers: 4, MaxHandCards: 6, StartingHandCards: 5}
	rules := query.GameRules{Match: match}
	reader := func(context.Context) (query.GameRules, error) { return rules, nil }

	got, err := query.NewGameRulesHandler(reader).Get(t.Context())

	require.NoError(t, err)
	require.Equal(t, rules, got)
}

// 投影がまだのとき、既定の値に戻さずエラーを返す
func TestGameRulesHandler_FailsWhenNotProjected(t *testing.T) {
	reader := func(context.Context) (query.GameRules, error) {
		return query.GameRules{}, query.ErrGameRulesNotProjected
	}

	_, err := query.NewGameRulesHandler(reader).Get(t.Context())

	require.ErrorIs(t, err, query.ErrGameRulesNotProjected)
}
