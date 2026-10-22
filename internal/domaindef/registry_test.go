package domaindef_test

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

// 集約を足したら、ここにその集約の Types を足す（マイグレーションの種別の足し忘れをテストで検出する）
func aggregates() []domain.AggregateTypes {
	return []domain.AggregateTypes{operator.Types(), creature.Types(), item.Types(), spell.Types(), presetdeck.Types(), starterdeckset.Types()}
}

func entityTypes() []domain.EntityType {
	var entityTypes []domain.EntityType
	for _, aggregate := range aggregates() {
		entityTypes = append(entityTypes, aggregate.EntityType)
	}
	return entityTypes
}

// テストのトランザクションの中だけで、DB に行を足す（コードに無い種別を作る）
func insert(t *testing.T, ctx context.Context, query string) {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	_, err = exec.ExecContext(ctx, query)
	require.NoError(t, err)
}

func TestVerifyEntityTypes_AllDeclarationsMatchMigrations(t *testing.T) {
	registry, err := domaindef.Load(rdbtest.Tx(t))
	require.NoError(t, err)

	require.NoError(t, registry.VerifyEntityTypes(entityTypes()...))
}

func TestVerifyEntityTypes_ReportsMismatchInBothDirections(t *testing.T) {
	ctx := rdbtest.Tx(t)
	insert(t, ctx, `INSERT INTO domain_entity_types (id, name) VALUES (999, 'domaindef_test')`)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)

	err = registry.VerifyEntityTypes(append(entityTypes(), domain.NewEntityType("unknown"))...)

	require.ErrorContains(t, err, "entity type is not in domain_entity_types: unknown")
	require.ErrorContains(t, err, "domain_entity_types has an entity type not declared in code: domaindef_test")
}

func TestVerifyEventTypes_AllDeclarationsMatchMigrations(t *testing.T) {
	registry, err := domaindef.Load(rdbtest.Tx(t))
	require.NoError(t, err)

	require.NoError(t, registry.VerifyEventTypes(aggregates()...))
}

// イベントの型を消した・名前を変えた後も、DB には種別が残る（過去のイベントを読むときに初めて失敗するのを、起動時に見つける）
func TestVerifyEventTypes_ReportsMismatchInBothDirections(t *testing.T) {
	ctx := rdbtest.Tx(t)
	insert(t, ctx, `INSERT INTO domain_event_types (id, entity_type_id, name)
		SELECT 999, id, 'RemovedEvent' FROM domain_entity_types WHERE name = 'operator'`)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	unknown := domain.AggregateTypes{EntityType: domain.NewEntityType("operator"), EventTypes: []domain.EventName{domain.NewEventName("UnknownEvent")}}

	err = registry.VerifyEventTypes(append(aggregates(), unknown)...)

	require.ErrorContains(t, err, "event type is not in domain_event_types: operator.UnknownEvent")
	require.ErrorContains(t, err, "domain_event_types has an event type not declared in code: operator.RemovedEvent")
}

// 一意な key の種別を足したら、ここにその種別を宣言するドメインの UniqueKeyTypes を足す
func uniqueKeyTypes() []domain.UniqueKeyType {
	return slices.Concat(operator.UniqueKeyTypes(), card.UniqueKeyTypes(), presetdeck.UniqueKeyTypes(), starterdeckset.UniqueKeyTypes())
}

func TestVerifyUniqueKeyTypes_AllKeyTypesMatchMigrations(t *testing.T) {
	registry, err := domaindef.Load(rdbtest.Tx(t))
	require.NoError(t, err)

	require.NoError(t, registry.VerifyUniqueKeyTypes(uniqueKeyTypes()...))
}

func TestVerifyUniqueKeyTypes_ReportsMismatchInBothDirections(t *testing.T) {
	ctx := rdbtest.Tx(t)
	insert(t, ctx, `INSERT INTO domain_unique_key_types (id, name) VALUES (999, 'domaindef_test.key')`)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)

	err = registry.VerifyUniqueKeyTypes(append(uniqueKeyTypes(), domain.NewUniqueKeyType("unknown.key_type"))...)

	require.ErrorContains(t, err, "unique key type is not in domain_unique_key_types: unknown.key_type")
	require.ErrorContains(t, err, "domain_unique_key_types has a unique key type not declared in code: domaindef_test.key")
}

// 参照の種別を足したら、ここにその種別を宣言するドメインの ReferenceTypes を足す
func referenceTypes() []domain.ReferenceType {
	return slices.Concat(presetdeck.ReferenceTypes(), starterdeckset.ReferenceTypes())
}

func TestVerifyReferenceTypes_AllReferenceTypesMatchMigrations(t *testing.T) {
	registry, err := domaindef.Load(rdbtest.Tx(t))
	require.NoError(t, err)

	require.NoError(t, registry.VerifyReferenceTypes(referenceTypes()...))
}

func TestVerifyReferenceTypes_ReportsMismatchInBothDirections(t *testing.T) {
	ctx := rdbtest.Tx(t)
	insert(t, ctx, `INSERT INTO domain_reference_types (id, name) VALUES (999, 'domaindef_test.reference')`)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)

	err = registry.VerifyReferenceTypes(append(referenceTypes(), domain.NewReferenceType("unknown.reference_type"))...)

	require.ErrorContains(t, err, "reference type is not in domain_reference_types: unknown.reference_type")
	require.ErrorContains(t, err, "domain_reference_types has a reference type not declared in code: domaindef_test.reference")
}
