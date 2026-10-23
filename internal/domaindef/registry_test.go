package domaindef_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

func TestVerifyEntityTypes_AllDeclarationsMatchMigrations(t *testing.T) {
	registry, err := domaindef.Load(rdbtest.Tx(t))
	require.NoError(t, err)

	require.NoError(t, registry.VerifyEntityTypes(entityTypes()...))
}

func TestVerifyEntityTypes_ReportsMismatchInBothDirections(t *testing.T) {
	ctx := isolated(t)
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
	ctx := isolated(t)
	insert(t, ctx, `INSERT INTO domain_event_types (id, entity_type_id, name)
		SELECT 999, id, 'RemovedEvent' FROM domain_entity_types WHERE name = 'operator'`)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	unknown := domain.AggregateTypes{EntityType: domain.NewEntityType("operator"), EventTypes: []domain.EventName{domain.NewEventName("UnknownEvent")}}

	err = registry.VerifyEventTypes(append(aggregates(), unknown)...)

	require.ErrorContains(t, err, "event type is not in domain_event_types: operator.UnknownEvent")
	require.ErrorContains(t, err, "domain_event_types has an event type not declared in code: operator.RemovedEvent")
}

func TestVerifyUniqueKeyTypes_AllKeyTypesMatchMigrations(t *testing.T) {
	registry, err := domaindef.Load(rdbtest.Tx(t))
	require.NoError(t, err)

	require.NoError(t, registry.VerifyUniqueKeyTypes(uniqueKeyTypes()...))
}

func TestVerifyUniqueKeyTypes_ReportsMismatchInBothDirections(t *testing.T) {
	ctx := isolated(t)
	insert(t, ctx, `INSERT INTO domain_unique_key_types (id, name) VALUES (999, 'domaindef_test.key')`)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)

	err = registry.VerifyUniqueKeyTypes(append(uniqueKeyTypes(), domain.NewUniqueKeyType("unknown.key_type"))...)

	require.ErrorContains(t, err, "unique key type is not in domain_unique_key_types: unknown.key_type")
	require.ErrorContains(t, err, "domain_unique_key_types has a unique key type not declared in code: domaindef_test.key")
}

func TestVerifyReferenceTypes_AllReferenceTypesMatchMigrations(t *testing.T) {
	registry, err := domaindef.Load(rdbtest.Tx(t))
	require.NoError(t, err)

	require.NoError(t, registry.VerifyReferenceTypes(referenceTypes()...))
}

func TestVerifyReferenceTypes_ReportsMismatchInBothDirections(t *testing.T) {
	ctx := isolated(t)
	insert(t, ctx, `INSERT INTO domain_reference_types (id, name) VALUES (999, 'domaindef_test.reference')`)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)

	err = registry.VerifyReferenceTypes(append(referenceTypes(), domain.NewReferenceType("unknown.reference_type"))...)

	require.ErrorContains(t, err, "reference type is not in domain_reference_types: unknown.reference_type")
	require.ErrorContains(t, err, "domain_reference_types has a reference type not declared in code: domaindef_test.reference")
}
