package domaindef_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

// 集約を足したら、ここにその集約の Types を足す（マイグレーションの種別の足し忘れをテストで検出する）
func TestVerify_AllAggregatesMatchMigrations(t *testing.T) {
	registry, err := domaindef.Load(rdbtest.Tx(t))
	require.NoError(t, err)

	require.NoError(t, registry.Verify(operator.Types(), creature.Types(), item.Types()))
}

func TestVerify_ReportsUnknownEntityAndEventTypes(t *testing.T) {
	registry, err := domaindef.Load(rdbtest.Tx(t))
	require.NoError(t, err)

	err = registry.Verify(
		domain.AggregateTypes{EntityType: domain.NewEntityType("unknown")},
		domain.AggregateTypes{EntityType: domain.NewEntityType("operator"), EventTypes: []domain.EventName{domain.NewEventName("UnknownEvent")}},
	)

	require.ErrorContains(t, err, "entity type not found: unknown")
	require.ErrorContains(t, err, "name=UnknownEvent")
}

// 一意な key の種別を足したら、ここにその種別を宣言するドメインの UniqueKeyTypes を足す
func TestVerifyUniqueKeyTypes_AllKeyTypesMatchMigrations(t *testing.T) {
	registry, err := domaindef.Load(rdbtest.Tx(t))
	require.NoError(t, err)

	require.NoError(t, registry.VerifyUniqueKeyTypes(slices.Concat(operator.UniqueKeyTypes(), card.UniqueKeyTypes())...))
}

func TestVerifyUniqueKeyTypes_ReportsUnknownKeyType(t *testing.T) {
	registry, err := domaindef.Load(rdbtest.Tx(t))
	require.NoError(t, err)

	err = registry.VerifyUniqueKeyTypes(domain.NewUniqueKeyType("unknown.key_type"))

	require.ErrorContains(t, err, "unique key type not found: unknown.key_type")
}
