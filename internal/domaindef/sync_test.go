package domaindef_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
)

func TestSync_ChangesNothingWhenDBMatchesCatalog(t *testing.T) {
	ctx := isolated(t)
	before := count(t, ctx, "domain_event_types")

	require.NoError(t, domaindef.Sync(ctx, domaindef.Types))
	require.NoError(t, domaindef.Sync(ctx, domaindef.Types))

	require.Equal(t, before, count(t, ctx, "domain_event_types"))
}

func TestSync_AddsTypesMissingFromDB(t *testing.T) {
	ctx := isolated(t)
	catalog := domaindef.Types
	catalog.EntityTypes = append(catalog.EntityTypes[:len(catalog.EntityTypes):len(catalog.EntityTypes)], domaindef.EntityTypeDef{
		ID: 9900, Name: "sync_test", Events: []domaindef.EventTypeDef{{ID: 9901, Name: "AddedEvent"}},
	})

	require.NoError(t, domaindef.Sync(ctx, catalog))

	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	entityTypeID, err := registry.EntityTypeID("sync_test")
	require.NoError(t, err)
	require.Equal(t, domaindef.NewEntityTypeID(9900), entityTypeID)
	eventTypeID, err := registry.EventTypeID(entityTypeID, "AddedEvent")
	require.NoError(t, err)
	require.Equal(t, domaindef.NewEventTypeID(9901), eventTypeID)
}

// 消さない。食い違いは足さずに止める（同じ id で名前が違う行・一覧に無い行）
func TestSync_ReportsRowsThatDifferFromCatalog(t *testing.T) {
	ctx := isolated(t)
	insert(t, ctx, `INSERT INTO domain_unique_key_types (id, name) VALUES (9900, 'not_in_catalog.key')`)
	catalog := domaindef.Types
	catalog.ReferenceTypes = []domaindef.NamedTypeDef{{ID: 1000, Name: "renamed.reference"}, catalog.ReferenceTypes[1]}
	catalog.QuotaTypes = []domaindef.NamedTypeDef{{ID: 1000, Name: "renamed.quota"}}

	err := domaindef.Sync(ctx, catalog)

	require.ErrorContains(t, err, "domain_unique_key_types has a row not in catalog: 9900: not_in_catalog.key")
	require.ErrorContains(t, err, "catalog has a row not in domain_reference_types: 1000: renamed.reference")
	require.ErrorContains(t, err, "domain_reference_types has a row not in catalog: 1000: preset_deck.card")
	require.ErrorContains(t, err, "catalog has a row not in domain_quota_types: 1000: renamed.quota")
	require.ErrorContains(t, err, "domain_quota_types has a row not in catalog: 1000: deck.per_player")
}
