package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
)

// このパッケージのテストは、テンプレートから複製した自分専用の DB で動かす（同時に呼ぶテストは commit が要るため、
// ほかのテストもそろえて、パッケージの中の DB の使い方を 1 つにする）
func isolatedDatabase(t *testing.T) context.Context {
	t.Helper()
	db, _ := rdbtest.NewDatabase(t)
	return rdb.WithDB(t.Context(), db)
}

func newPlayerHandler(t *testing.T, ctx context.Context) *command.PlayerHandler {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewPlayerHandler(
		eventstore.New(registry, player.NewID, player.NewEventType, player.InitialState, player.Evolve),
		identifier.New(registry, player.NewID),
		constraint.UpdatePlayerSubject,
		constraint.FindPlayerBySubject,
		func() time.Time { return time.Now().UTC() },
	)
}

// 認証ミドルウェアと同じく、game の JWT（iss は diecardomain-game）の (iss, sub) を context に載せる
func authenticate(t *testing.T, ctx context.Context, commands *command.PlayerHandler, subject string) context.Context {
	t.Helper()
	authenticated, err := commands.Authenticate(ctx, "diecardomain-game", subject)
	require.NoError(t, err)
	return authenticated
}

// プレイヤーのイベントの数
func countEvents(t *testing.T, ctx context.Context) int {
	t.Helper()
	return countEventsOf(t, ctx, 1600)
}

// 集約の種別の id（internal/domaindef/catalog.go）のイベントの数
func countEventsOf(t *testing.T, ctx context.Context, entityTypeID int) int {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var count int
	require.NoError(t, exec.QueryRowContext(ctx,
		`SELECT count(*) FROM domain_events WHERE entity_type_id = $1`, entityTypeID).Scan(&count))
	return count
}

func newStarterGrantHandler(t *testing.T, ctx context.Context, getStarterDeckSet master.GetStarterDeckSetFunc) *command.StarterGrantHandler {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	now := func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }
	return command.NewStarterGrantHandler(
		eventstore.New(registry, startergrant.NewID, startergrant.NewEventType, startergrant.InitialState, startergrant.Evolve),
		identifier.New(registry, startergrant.NewID),
		constraint.UpdateStarterGrantPlayer,
		getStarterDeckSet,
		newCollectionHandler(t, ctx),
		newDeckHandler(t, ctx),
		now,
	)
}

func newCollectionHandler(t *testing.T, ctx context.Context) *command.CollectionHandler {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewCollectionHandler(
		eventstore.New(registry, collection.NewID, collection.NewEventType, collection.InitialState, collection.Evolve),
		identifier.New(registry, collection.NewID),
		constraint.UpdateCollectionOwner,
		func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	)
}

func newDeckHandler(t *testing.T, ctx context.Context) *command.DeckHandler {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewDeckHandler(
		eventstore.New(registry, deck.NewID, deck.NewEventType, deck.InitialState, deck.Evolve),
		identifier.New(registry, deck.NewID),
		constraint.UpdateDeckStarter,
		func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	)
}

// 配布の対象のプレイヤーの内部 ID（プレイヤーのイベントは作らず、識別子だけ発行する）
func issuePlayerID(t *testing.T, ctx context.Context) int64 {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, _, err := identifier.New(registry, player.NewID).Issue(ctx)
	require.NoError(t, err)
	return id.Int64()
}

// 配布の状態（プレイヤーの key から配布を引いて読む）
func loadStarterGrant(t *testing.T, ctx context.Context, playerID int64) startergrant.State {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	owner, err := player.NewID(playerID)
	require.NoError(t, err)
	id, err := startergrant.NewID(constrainttest.Owner(t, ctx, startergrant.PlayerKey(owner)))
	require.NoError(t, err)
	state, err := eventstore.New(registry, startergrant.NewID, startergrant.NewEventType, startergrant.InitialState, startergrant.Evolve).Load(ctx, id)
	require.NoError(t, err)
	return state
}
