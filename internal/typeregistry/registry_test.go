package typeregistry_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/typeregistry"
)

// 集約を足したら、ここにその集約の Types を足す（マイグレーションの種別の足し忘れをテストで検出する）
func TestVerify_AllAggregatesMatchMigrations(t *testing.T) {
	registry, err := typeregistry.Load(rdbtest.Tx(t))
	require.NoError(t, err)

	require.NoError(t, registry.Verify(operator.Types()))
}

func TestVerify_ReportsUnknownEntityAndEventTypes(t *testing.T) {
	registry, err := typeregistry.Load(rdbtest.Tx(t))
	require.NoError(t, err)

	err = registry.Verify(
		domain.AggregateTypes{EntityType: "unknown"},
		domain.AggregateTypes{EntityType: "operator", EventTypes: []string{"UnknownEvent"}},
	)

	require.ErrorContains(t, err, "entity type not found: unknown")
	require.ErrorContains(t, err, "name=UnknownEvent")
}
