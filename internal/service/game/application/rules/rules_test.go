package rules_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/rules"
)

func TestDefaults_ReturnsLimitOfQuotaType(t *testing.T) {
	deckType := domain.NewQuotaType("test.deck")
	limit, err := domain.NewQuotaLimit(7)
	require.NoError(t, err)

	got, err := rules.Defaults(map[domain.QuotaType]domain.QuotaLimit{deckType: limit})(t.Context(), deckType)

	require.NoError(t, err)
	require.Equal(t, limit, got)
}

func TestDefaults_RejectsQuotaTypeWithoutDefault(t *testing.T) {
	_, err := rules.Defaults(map[domain.QuotaType]domain.QuotaLimit{})(t.Context(), domain.NewQuotaType("test.unknown"))

	require.Error(t, err)
}
