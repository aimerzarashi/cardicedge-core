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
	owner := playertest.ID(t, issuePlayerID(t, ctx))
	grant := startergranttest.ID(t, 10)
	cards := []startergrant.CardCount{startergranttest.CardCount(t, cardA, 4), startergranttest.CardCount(t, cardB, 3)}

	require.NoError(t, newCollectionHandler(t, ctx).Create(ctx, owner, grant, cards))

	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := collection.NewID(constrainttest.Owner(t, ctx, collection.OwnerKey(owner)))
	require.NoError(t, err)
	state, err := eventstore.New(registry, collection.NewID, collection.NewEventType, collection.InitialState, collection.Evolve).Load(ctx, id)
	require.NoError(t, err)
	created, ok := state.(collection.Collection)
	require.True(t, ok)
	require.Equal(t, owner, created.Owner)
	require.Equal(t, grant, created.Grant)
	require.Equal(t, cards, created.Cards)
}

// プレイヤーに所持カードは 1 つ。2 回目はエラーにせず何もしない（ポリシーのやり直しで同じ作成が来ても害が無いように）
func TestCreateCollection_DoesNothingWhenOwnerAlreadyHasCollection(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newCollectionHandler(t, ctx)
	owner := playertest.ID(t, issuePlayerID(t, ctx))
	cards := []startergrant.CardCount{startergranttest.CardCount(t, cardA, 1)}
	require.NoError(t, handler.Create(ctx, owner, startergranttest.ID(t, 10), cards))

	err := handler.Create(ctx, owner, startergranttest.ID(t, 10), cards)

	require.NoError(t, err)
	require.Equal(t, 1, countEventsOf(t, ctx, collectionTypeID))
}
