package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query"
)

func TestStarterDeckSetHandler_ReturnsEmptyBeforeSet(t *testing.T) {
	get := func(context.Context) (query.StarterDeckSet, bool, error) {
		return query.StarterDeckSet{}, false, nil
	}

	set, err := query.NewStarterDeckSetHandler(get, nil).Get(t.Context())

	require.NoError(t, err)
	require.Equal(t, query.StarterDeckSet{Decks: []query.StarterDeck{}}, set)
}

// 写した後に元が変わっていれば true。同じ revision なら false。元が消されていれば比べる相手が無いので false
func TestStarterDeckSetHandler_ComparesSourceWithCurrentPresetDeck(t *testing.T) {
	unchanged, changed, deleted := uuid.New(), uuid.New(), uuid.New()
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	deck := func(name string, presetDeckID uuid.UUID) query.StarterDeck {
		source := query.StarterDeckSource{PresetDeckID: presetDeckID, Revision: 5}
		return query.StarterDeck{Name: name, Source: source}
	}
	decks := []query.StarterDeck{deck("unchanged", unchanged), deck("changed", changed), deck("deleted", deleted)}
	stored := query.StarterDeckSet{Revision: 2, UpdatedAt: &at, Decks: decks}
	get := func(context.Context) (query.StarterDeckSet, bool, error) {
		return stored, true, nil
	}
	revisions := func(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error) {
		require.ElementsMatch(t, []uuid.UUID{unchanged, changed, deleted}, ids)
		return map[uuid.UUID]int64{unchanged: 5, changed: 6}, nil
	}

	set, err := query.NewStarterDeckSetHandler(get, revisions).Get(t.Context())

	require.NoError(t, err)
	changedSinceCopied := map[string]bool{}
	for _, deck := range set.Decks {
		changedSinceCopied[deck.Name] = deck.Source.ChangedSinceCopied
	}
	require.Equal(t, map[string]bool{"unchanged": false, "changed": true, "deleted": false}, changedSinceCopied)
}
