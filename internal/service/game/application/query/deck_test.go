package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

func TestDeckHandler_ListByPlayer(t *testing.T) {
	first := query.Deck{ID: uuid.New(), Name: "火"}
	second := query.Deck{ID: uuid.New(), Name: "水"}
	tests := []struct {
		name string
		rows []query.Deck
		want []query.Deck
	}{
		{name: "in_given_order", rows: []query.Deck{first, second}, want: []query.Deck{first, second}},
		{name: "none_is_empty_list", rows: nil, want: []query.Deck{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := func(context.Context, uuid.UUID) ([]query.Deck, error) { return tt.rows, nil }

			got, err := query.NewDeckHandler(list, nil).ListByPlayer(t.Context(), uuid.New())

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestDeck_TotalCountSumsCardCounts(t *testing.T) {
	deck := query.Deck{Cards: []query.DeckCard{{Count: 3}, {Count: 2}}}

	require.Equal(t, 5, deck.TotalCount())
}

func TestDeck_TypeCountsSumsCardCountsPerType(t *testing.T) {
	tests := []struct {
		name  string
		cards []query.DeckCard
		want  query.DeckTypeCounts
	}{
		{name: "no_cards", cards: nil, want: query.NewDeckTypeCounts(0, 0, 0)},
		{name: "sums_counts_not_kinds", cards: []query.DeckCard{
			{Type: query.CardTypeCreature, Count: 3},
			{Type: query.CardTypeCreature, Count: 2},
			{Type: query.CardTypeItem, Count: 4},
			{Type: query.CardTypeSpell, Count: 1},
		}, want: query.NewDeckTypeCounts(5, 4, 1)},
		{name: "absent_types_are_zero", cards: []query.DeckCard{{Type: query.CardTypeSpell, Count: 2}}, want: query.NewDeckTypeCounts(0, 0, 2)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, query.Deck{Cards: tt.cards}.TypeCounts())
		})
	}
}

func TestDeckHandler_GetPassesPlayerAndDeckToReader(t *testing.T) {
	playerID, deckID := uuid.New(), uuid.New()
	want := query.Deck{ID: deckID, Name: "火"}
	get := func(_ context.Context, gotPlayer uuid.UUID, gotDeck uuid.UUID) (query.Deck, error) {
		require.Equal(t, playerID, gotPlayer)
		require.Equal(t, deckID, gotDeck)
		return want, nil
	}

	got, err := query.NewDeckHandler(nil, get).Get(t.Context(), playerID, deckID)

	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestDeckHandler_GetReturnsNotFoundAsIs(t *testing.T) {
	get := func(context.Context, uuid.UUID, uuid.UUID) (query.Deck, error) {
		return query.Deck{}, query.ErrDeckNotFound
	}

	_, err := query.NewDeckHandler(nil, get).Get(t.Context(), uuid.New(), uuid.New())

	require.ErrorIs(t, err, query.ErrDeckNotFound)
}
