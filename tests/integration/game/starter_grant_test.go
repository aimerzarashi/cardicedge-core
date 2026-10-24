package game_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

// 配布が終わったか（配布の集約に CompletedEvent があるか）。配布の ID は、プレイヤーの一意な key から引く
const starterGrantCompletedQuery = `
	SELECT EXISTS (
		SELECT 1
		FROM domain_unique_keys u
		JOIN domain_unique_key_types t ON t.id = u.unique_key_type_id
		JOIN domain_events e ON e.entity_id = u.entity_id
		JOIN domain_event_types et ON et.id = e.event_type_id
		WHERE t.name = 'starter_grant.player' AND u.key = $1 AND et.name = 'CompletedEvent'
	)
`

func waitStarterGrantCompleted(t *testing.T, db *sql.DB, owner player.ID) {
	t.Helper()
	ctx := rdb.WithDB(t.Context(), db)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var completed bool
		err := db.QueryRowContext(ctx, starterGrantCompletedQuery, startergrant.PlayerKey(owner).Value()).Scan(&completed)
		assert.NoError(c, err)
		assert.True(c, completed)
	})
}

// 登録の結果の公開 ID から、game の DB の内部 ID を引く
func resolvePlayer(t *testing.T, db *sql.DB, publicID string) player.ID {
	t.Helper()
	ctx := rdb.WithDB(t.Context(), db)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := identifier.New(registry, player.NewID).Resolve(ctx, uuid.MustParse(publicID))
	require.NoError(t, err)
	return id
}

// 配られた所持カードとデッキ 2 つが、マスタの中身どおりか確かめる
func requireDistributed(t *testing.T, db *sql.DB, m testhelper.Master, owner player.ID) {
	t.Helper()
	ctx := rdb.WithDB(t.Context(), db)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)

	grantID, err := startergrant.NewID(constrainttest.Owner(t, ctx, startergrant.PlayerKey(owner)))
	require.NoError(t, err)
	grant, err := eventstore.New(registry, startergrant.NewID, startergrant.NewEventType, startergrant.InitialState, startergrant.Evolve).Load(ctx, grantID)
	require.NoError(t, err)
	require.IsType(t, startergrant.CompletedStarterGrant{}, grant)

	collectionID, err := collection.NewID(constrainttest.Owner(t, ctx, collection.OwnerKey(owner)))
	require.NoError(t, err)
	owned, err := eventstore.New(registry, collection.NewID, collection.NewEventType, collection.InitialState, collection.Evolve).Load(ctx, collectionID)
	require.NoError(t, err)
	// 並びはカードの ID の順（ランダムな ID のため、テストでは順を問わない）
	require.ElementsMatch(t, []startergrant.CardCount{
		startergranttest.CardCount(t, m.CardA, 4),
		startergranttest.CardCount(t, m.CardB, 3),
		startergranttest.CardCount(t, m.CardC, 2),
	}, owned.(collection.Collection).Cards)

	first := loadDeck(t, ctx, registry, grantID, 1)
	second := loadDeck(t, ctx, registry, grantID, 2)
	require.Equal(t, "火", first.Name.String())
	require.Equal(t, []startergrant.CardCount{startergranttest.CardCount(t, m.CardA, 4), startergranttest.CardCount(t, m.CardB, 1)}, first.Cards)
	require.Equal(t, "水", second.Name.String())
	require.Equal(t, []startergrant.CardCount{
		startergranttest.CardCount(t, m.CardA, 2), startergranttest.CardCount(t, m.CardB, 3), startergranttest.CardCount(t, m.CardC, 2),
	}, second.Cards)
}

func loadDeck(t *testing.T, ctx context.Context, registry *domaindef.Registry, grantID startergrant.ID, number int) deck.Deck {
	t.Helper()
	id, err := deck.NewID(constrainttest.Owner(t, ctx, deck.StarterKey(grantID, startergranttest.DeckNumber(t, number))))
	require.NoError(t, err)
	state, err := eventstore.New(registry, deck.NewID, deck.NewEventType, deck.InitialState, deck.Evolve).Load(ctx, id)
	require.NoError(t, err)
	return state.(deck.Deck)
}

func TestRegisterPlayer_GrantsStarterCollectionAndDecks(t *testing.T) {
	t.Parallel()

	owner := resolvePlayer(t, gameDB, registeredPlayerID(t, registerPlayer(t, newPlayerToken(t))))

	waitStarterGrantCompleted(t, gameDB, owner)
	requireDistributed(t, gameDB, master, owner)
}

// マスタの有無を変えるため、共有のアプリではなく、独立したアプリ（自分だけの DB）で動かす
// （共有のアプリのマスタを消すと、並列に動くほかのテストの配布が止まる）
func TestRegisterPlayer_GrantsStarterAfterMasterIsPublished(t *testing.T) {
	t.Parallel()
	app, err := testhelper.StartApp(t.Context())
	require.NoError(t, err)
	t.Cleanup(app.Close)
	isolated := testhelper.GraphQLClient{URL: app.URL + "/game/graphql"}
	claims := idptest.ValidClaims("player-" + testhelper.UniqueID(t))
	claims.Issuer = idptest.GameIssuer
	registered := isolated.Data(t, app.Key.Sign(t, claims), registerPlayerMutation, nil, "registerPlayer")
	owner := resolvePlayer(t, app.GameDB, registeredPlayerID(t, registered))
	ctx := rdb.WithDB(t.Context(), app.GameDB)
	// マスタが無いため、配布を始められずに失敗の記録が残る（やり直しの間隔ごとに繰り返す）のを待ってから公開する
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var failures int
		assert.NoError(c, app.GameDB.QueryRowContext(ctx, `SELECT count(*) FROM event_subscriber_failures`).Scan(&failures))
		assert.Positive(c, failures)
	})

	published, err := testhelper.PublishMaster(ctx, app.GameDB)
	require.NoError(t, err)

	waitStarterGrantCompleted(t, app.GameDB, owner)
	requireDistributed(t, app.GameDB, published, owner)
}
