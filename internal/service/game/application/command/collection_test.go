package command_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
)

func TestCreateCollection_CreatesCollectionOfOwnerWithGivenCards(t *testing.T) {
	ctx := isolatedDatabase(t)
	playerID := playertest.ID(t, issuePlayerID(t, ctx))
	starterGrantID := startergranttest.ID(t, 10)
	cards := []startergrant.CardCount{startergranttest.CardCount(t, cardA, 4), startergranttest.CardCount(t, cardB, 3)}

	require.NoError(t, newCollectionHandler(t, ctx).Create(ctx, playerID, starterGrantID, cards))

	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := collection.NewID(constrainttest.Owner(t, ctx, collection.PlayerKey(playerID)))
	require.NoError(t, err)
	state, err := eventstore.New(registry, collection.NewID, collection.NewEventType, collection.InitialState, collection.Evolve).Load(ctx, id)
	require.NoError(t, err)
	created, ok := state.(collection.Collection)
	require.True(t, ok)
	require.Equal(t, playerID, created.Player)
	require.Equal(t, starterGrantID, created.StarterGrant)
	require.Equal(t, cards, created.Cards)
}

// プレイヤーに所持カードは 1 つ。2 回目はエラーにせず何もしない（ポリシーのやり直しで同じ作成が来ても害が無いように）
func TestCreateCollection_DoesNothingWhenOwnerAlreadyHasCollection(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newCollectionHandler(t, ctx)
	playerID := playertest.ID(t, issuePlayerID(t, ctx))
	cards := []startergrant.CardCount{startergranttest.CardCount(t, cardA, 1)}
	require.NoError(t, handler.Create(ctx, playerID, startergranttest.ID(t, 10), cards))

	err := handler.Create(ctx, playerID, startergranttest.ID(t, 10), cards)

	require.NoError(t, err)
	require.Equal(t, 1, countEventsOf(t, ctx, collectionTypeID))
}
