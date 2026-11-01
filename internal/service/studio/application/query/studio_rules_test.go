package query_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
)

func TestStudioRulesHandler_ReturnsRulesFromReader(t *testing.T) {
	board := query.BoardRules{MinSize: 2, MaxSize: 16}
	rules := query.StudioRules{Board: board}
	reader := func(context.Context) (query.StudioRules, error) { return rules, nil }

	got, err := query.NewStudioRulesHandler(reader).Get(t.Context())

	require.NoError(t, err)
	require.Equal(t, rules, got)
}

// 投影がまだのとき、既定の値に戻さずエラーを返す
func TestStudioRulesHandler_FailsWhenNotProjected(t *testing.T) {
	reader := func(context.Context) (query.StudioRules, error) {
		return query.StudioRules{}, query.ErrStudioRulesNotProjected
	}

	_, err := query.NewStudioRulesHandler(reader).Get(t.Context())

	require.ErrorIs(t, err, query.ErrStudioRulesNotProjected)
}
