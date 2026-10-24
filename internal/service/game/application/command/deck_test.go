package command_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck/decktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
)

func TestCreateDeck_CreatesDeckOfOwnerWithNameAndCardsOfSource(t *testing.T) {
	ctx := isolatedDatabase(t)
	owner := playertest.ID(t, issuePlayerID(t, ctx))
	grant := startergranttest.ID(t, 10)
	number := startergranttest.DeckNumber(t, 2)
	source := startergranttest.Deck(t, "水", startergranttest.CardCount(t, cardA, 2), startergranttest.CardCount(t, cardC, 2))

	require.NoError(t, newDeckHandler(t, ctx).Create(ctx, owner, grant, number, source))

	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := deck.NewID(constrainttest.Owner(t, ctx, deck.StarterKey(grant, number)))
	require.NoError(t, err)
	state, err := eventstore.New(registry, deck.NewID, deck.NewEventType, deck.InitialState, deck.Evolve).Load(ctx, id)
	require.NoError(t, err)
	created, ok := state.(deck.Deck)
	require.True(t, ok)
	require.Equal(t, owner, created.Owner)
	require.Equal(t, decktest.Name(t, "水"), created.Name)
	require.Equal(t, source.Cards, created.Cards)
	require.Equal(t, grant, created.Grant)
	require.Equal(t, number, created.Number)
}

// 同じ配布の同じ番号のデッキは 1 つ。2 回目はエラーにせず何もしない（ポリシーのやり直しで同じ作成が来ても害が無いように）
func TestCreateDeck_DoesNothingWhenStarterDeckAlreadyExists(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newDeckHandler(t, ctx)
	owner := playertest.ID(t, issuePlayerID(t, ctx))
	grant := startergranttest.ID(t, 10)
	number := startergranttest.DeckNumber(t, 1)
	source := startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4))
	require.NoError(t, handler.Create(ctx, owner, grant, number, source))

	err := handler.Create(ctx, owner, grant, number, source)

	require.NoError(t, err)
	require.Equal(t, 1, countEventsOf(t, ctx, deckTypeID))
}
