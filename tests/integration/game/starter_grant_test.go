package game_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
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

func waitStarterGrantCompleted(t *testing.T, db *sql.DB, playerID player.ID) {
	t.Helper()
	ctx := rdb.WithDB(t.Context(), db)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var completed bool
		err := db.QueryRowContext(ctx, starterGrantCompletedQuery, startergrant.PlayerKey(playerID).Value()).Scan(&completed)
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

// 配られた所持カードとデッキ 2 つ（公開中のスターターデッキだけ）が、マスタの中身どおりか確かめる
func requireDistributed(t *testing.T, db *sql.DB, m testhelper.Master, playerID player.ID) {
	t.Helper()
	ctx := rdb.WithDB(t.Context(), db)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)

	grantID, err := startergrant.NewID(constrainttest.Owner(t, ctx, startergrant.PlayerKey(playerID)))
	require.NoError(t, err)
	grant, err := eventstore.New(registry, startergrant.NewID, startergrant.NewEventType, startergrant.InitialState, startergrant.Evolve).Load(ctx, grantID)
	require.NoError(t, err)
	require.IsType(t, startergrant.CompletedStarterGrant{}, grant)

	collectionID, err := collection.NewID(constrainttest.Owner(t, ctx, collection.PlayerKey(playerID)))
	require.NoError(t, err)
	var raw []byte
	require.NoError(t, db.QueryRowContext(ctx, `SELECT payload FROM domain_events WHERE entity_id = $1 AND aggregate_version = 1`, collectionID.Int64()).Scan(&raw))
	var owned collection.CreatedEvent
	require.NoError(t, json.Unmarshal(raw, &owned))
	// 並びはカードの ID の順（ランダムな ID のため、テストでは順を問わない）
	require.ElementsMatch(t, []startergrant.CardCount{
		startergranttest.CardCount(t, m.CardA, 4),
		startergranttest.CardCount(t, m.CardB, 3),
		startergranttest.CardCount(t, m.CardC, 2),
	}, owned.Cards)

	var deckCount int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM domain_unique_keys u JOIN domain_unique_key_types t ON t.id = u.unique_key_type_id
		WHERE t.name = 'deck.starter' AND u.key LIKE $1`, strconv.FormatInt(grantID.Int64(), 10)+":%").Scan(&deckCount))
	require.Equal(t, 2, deckCount)
	first := loadDeck(t, ctx, grantID, 1)
	second := loadDeck(t, ctx, grantID, 2)
	require.Equal(t, "火", first.Name.String())
	require.Equal(t, []startergrant.CardCount{startergranttest.CardCount(t, m.CardA, 4), startergranttest.CardCount(t, m.CardB, 1)}, first.Cards)
	require.Equal(t, "水", second.Name.String())
	require.Equal(t, []startergrant.CardCount{
		startergranttest.CardCount(t, m.CardA, 2), startergranttest.CardCount(t, m.CardB, 3), startergranttest.CardCount(t, m.CardC, 2),
	}, second.Cards)
}

// State は名前・カードを持たない（Decide が使わないため）ので、作成のイベントの payload から読む
func loadDeck(t *testing.T, ctx context.Context, grantID startergrant.ID, number int) deck.CreatedEvent {
	t.Helper()
	id, err := deck.NewID(constrainttest.Owner(t, ctx, deck.StarterKey(grantID, startergranttest.DeckNumber(t, number))))
	require.NoError(t, err)
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var raw []byte
	require.NoError(t, exec.QueryRowContext(ctx, `SELECT payload FROM domain_events WHERE entity_id = $1 AND aggregate_version = 1`, id.Int64()).Scan(&raw))
	var created deck.CreatedEvent
	require.NoError(t, json.Unmarshal(raw, &created))
	return created
}

func TestRegisterPlayer_GrantsStarterCollectionAndDecks(t *testing.T) {
	t.Parallel()

	playerID := resolvePlayer(t, gameDB, registeredPlayerID(t, registerPlayer(t, newPlayerToken(t))))

	waitStarterGrantCompleted(t, gameDB, playerID)
	requireDistributed(t, gameDB, master, playerID)
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
	playerID := resolvePlayer(t, app.GameDB, registeredPlayerID(t, registered))
	ctx := rdb.WithDB(t.Context(), app.GameDB)
	// マスタが無いため、配布を始められずに失敗の記録が残る（やり直しの間隔ごとに繰り返す）のを待ってから公開する
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var failures int
		assert.NoError(c, app.GameDB.QueryRowContext(ctx, `SELECT count(*) FROM event_subscriber_failures`).Scan(&failures))
		assert.Positive(c, failures)
	})

	published, err := testhelper.PublishMaster(ctx, app.GameDB)
	require.NoError(t, err)

	waitStarterGrantCompleted(t, app.GameDB, playerID)
	requireDistributed(t, app.GameDB, published, playerID)
}

// game_rules が写されていないと、マスタがあっても配布は終わらない（ドメインの既定の値で黙って動かない）。
// 独立したアプリ（自分だけの DB）で、写す前は終わらず、写した後に終わることを確かめる
func TestRegisterPlayer_WaitsForGameRulesToBePublished(t *testing.T) {
	t.Parallel()
	app, err := testhelper.StartApp(t.Context())
	require.NoError(t, err)
	t.Cleanup(app.Close)
	isolated := testhelper.GraphQLClient{URL: app.URL + "/game/graphql"}
	claims := idptest.ValidClaims("player-" + testhelper.UniqueID(t))
	claims.Issuer = idptest.GameIssuer
	registered := isolated.Data(t, app.Key.Sign(t, claims), registerPlayerMutation, nil, "registerPlayer")
	playerID := resolvePlayer(t, app.GameDB, registeredPlayerID(t, registered))
	ctx := rdb.WithDB(t.Context(), app.GameDB)
	published, err := testhelper.PublishMasterWithoutGameRules(ctx, app.GameDB)
	require.NoError(t, err)
	// 配布は始まるが、デッキを作る段で上限を読めず、失敗の記録が残る（やり直しの間隔ごとに繰り返す）
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var failures int
		assert.NoError(c, app.GameDB.QueryRowContext(ctx, `SELECT count(*) FROM event_subscriber_failures`).Scan(&failures))
		assert.Positive(c, failures)
	})
	var decks int
	require.NoError(t, app.GameDB.QueryRowContext(ctx, `SELECT count(*) FROM domain_events WHERE entity_type_id = 1900`).Scan(&decks))
	require.Zero(t, decks)

	require.NoError(t, testhelper.PublishGameRules(ctx, app.GameDB))

	waitStarterGrantCompleted(t, app.GameDB, playerID)
	requireDistributed(t, app.GameDB, published, playerID)
}

// 公開中のスターターデッキが 1 つも無ければ（準備中・使用不可だけでは）配布は始まらず、公開中のものが写されると配られる
func TestRegisterPlayer_WaitsForPublishedStarterDeck(t *testing.T) {
	t.Parallel()
	app, err := testhelper.StartApp(t.Context())
	require.NoError(t, err)
	t.Cleanup(app.Close)
	isolated := testhelper.GraphQLClient{URL: app.URL + "/game/graphql"}
	claims := idptest.ValidClaims("player-" + testhelper.UniqueID(t))
	claims.Issuer = idptest.GameIssuer
	registered := isolated.Data(t, app.Key.Sign(t, claims), registerPlayerMutation, nil, "registerPlayer")
	playerID := resolvePlayer(t, app.GameDB, registeredPlayerID(t, registered))
	ctx := rdb.WithDB(t.Context(), app.GameDB)
	published, err := testhelper.PublishMasterWithoutPublishedStarterDecks(ctx, app.GameDB)
	require.NoError(t, err)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var failures int
		assert.NoError(c, app.GameDB.QueryRowContext(ctx, `SELECT count(*) FROM event_subscriber_failures`).Scan(&failures))
		assert.Positive(c, failures)
	})
	var grants int
	require.NoError(t, app.GameDB.QueryRowContext(ctx, `SELECT count(*) FROM domain_events WHERE entity_type_id = 1700`).Scan(&grants))
	require.Zero(t, grants)

	require.NoError(t, testhelper.PublishStarterDecks(ctx, app.GameDB, published))

	waitStarterGrantCompleted(t, app.GameDB, playerID)
	requireDistributed(t, app.GameDB, published, playerID)
}

const registerPlayerStarterGrantMutation = `mutation {
	registerPlayer {
		... on RegisterPlayerSuccess { player { starterGrant } }
	}
}`

const meStarterGrantQuery = `{
	me {
		... on Player { starterGrant }
	}
}`

func starterGrantOf(t *testing.T, c testhelper.GraphQLClient, token, query, field string) string {
	t.Helper()
	data := c.Data(t, token, query, nil, field)
	if player, ok := data["player"].(map[string]any); ok {
		data = player
	}
	return data["starterGrant"].(string)
}

func TestMe_ReturnsCompletedStarterGrantAfterGranted(t *testing.T) {
	t.Parallel()
	token := newPlayerToken(t)
	playerID := resolvePlayer(t, gameDB, registeredPlayerID(t, registerPlayer(t, token)))

	waitStarterGrantCompleted(t, gameDB, playerID)

	testhelper.Eventually(t, func(c *assert.CollectT) {
		assert.Equal(c, "COMPLETED", starterGrantOf(t, client, token, meStarterGrantQuery, "me"))
	})
}

// マスタが無いと配布は始まらない。共有のアプリは公開済みなので、独立したアプリ（自分だけの DB）で確かめる
func TestMe_ReturnsInProgressStarterGrantBeforeGranted(t *testing.T) {
	t.Parallel()
	app, err := testhelper.StartApp(t.Context())
	require.NoError(t, err)
	t.Cleanup(app.Close)
	isolated := testhelper.GraphQLClient{URL: app.URL + "/game/graphql"}
	claims := idptest.ValidClaims("player-" + testhelper.UniqueID(t))
	claims.Issuer = idptest.GameIssuer
	token := app.Key.Sign(t, claims)

	require.Equal(t, "IN_PROGRESS", starterGrantOf(t, isolated, token, registerPlayerStarterGrantMutation, "registerPlayer"))
	require.Equal(t, "IN_PROGRESS", starterGrantOf(t, isolated, token, meStarterGrantQuery, "me"))

	_, err = testhelper.PublishMaster(rdb.WithDB(t.Context(), app.GameDB), app.GameDB)
	require.NoError(t, err)
	testhelper.Eventually(t, func(c *assert.CollectT) {
		assert.Equal(c, "COMPLETED", starterGrantOf(t, isolated, token, meStarterGrantQuery, "me"))
	})
}
