package command_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection/collectiontest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

func TestCreateCollection_CreatesCollectionOfOwnerWithGivenCards(t *testing.T) {
	ctx := isolatedDatabase(t)
	playerID := playertest.ID(t, issuePlayerID(t, ctx))
	starterGrantID := startergranttest.ID(t, 10)
	cards := []collection.Card{collectiontest.Card(t, cardA, 4), collectiontest.Card(t, cardB, 3)}

	require.NoError(t, newCollectionHandler(t, ctx).Create(ctx, playerID, starterGrantID, cards))

	// State は持ち主・配布・カードを持たない（Decide が使わないため）ので、作成のイベントの payload から読む
	id, err := collection.NewID(constrainttest.Owner(t, ctx, collection.PlayerKey(playerID)))
	require.NoError(t, err)
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var raw []byte
	require.NoError(t, exec.QueryRowContext(ctx, `SELECT payload FROM domain_events WHERE entity_id = $1 AND aggregate_version = 1`, id.Int64()).Scan(&raw))
	var created collection.CreatedEvent
	require.NoError(t, json.Unmarshal(raw, &created))
	require.Equal(t, playerID, created.Player)
	require.Equal(t, starterGrantID, created.StarterGrant)
	require.Equal(t, cards, created.Cards)
}

// プレイヤーに所持カードは 1 つ。2 回目はエラーにせず何もしない（ポリシーのやり直しで同じ作成が来ても害が無いように）
func TestCreateCollection_DoesNothingWhenOwnerAlreadyHasCollection(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newCollectionHandler(t, ctx)
	playerID := playertest.ID(t, issuePlayerID(t, ctx))
	cards := []collection.Card{collectiontest.Card(t, cardA, 1)}
	require.NoError(t, handler.Create(ctx, playerID, startergranttest.ID(t, 10), cards))

	err := handler.Create(ctx, playerID, startergranttest.ID(t, 10), cards)

	require.NoError(t, err)
	require.Equal(t, 1, countEventsOf(t, ctx, collectionTypeID))
}
