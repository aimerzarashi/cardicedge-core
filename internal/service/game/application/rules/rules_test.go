package rules_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/rules"
)

func published(rules gamerules.Rules) rules.GetRulesFunc {
	return func(context.Context) (gamerules.Rules, bool, error) {
		return rules, true, nil
	}
}

func TestFromPublished_ReturnsMaxDecksForDeckQuota(t *testing.T) {
	values := gamerules.Defaults()
	values.Player.MaxDecks = 7

	got, err := rules.FromPublished(published(values))(t.Context(), deck.PlayerQuotaType)

	require.NoError(t, err)
	require.Equal(t, 7, got.Int())
}

func TestFromPublished_RejectsQuotaTypeWithoutRule(t *testing.T) {
	_, err := rules.FromPublished(published(gamerules.Defaults()))(t.Context(), domain.NewQuotaType("test.unknown"))

	require.Error(t, err)
}

// 既定の値に黙って戻さない
func TestFromPublished_RejectsWhenNotPublished(t *testing.T) {
	notFound := func(context.Context) (gamerules.Rules, bool, error) {
		return gamerules.Rules{}, false, nil
	}

	_, err := rules.FromPublished(notFound)(t.Context(), deck.PlayerQuotaType)

	require.ErrorIs(t, err, rules.ErrNotPublished)
}

func TestFromPublished_PassesThroughReadError(t *testing.T) {
	failed := errors.New("read failed")
	get := func(context.Context) (gamerules.Rules, bool, error) {
		return gamerules.Rules{}, false, failed
	}

	_, err := rules.FromPublished(get)(t.Context(), deck.PlayerQuotaType)

	require.ErrorIs(t, err, failed)
}
