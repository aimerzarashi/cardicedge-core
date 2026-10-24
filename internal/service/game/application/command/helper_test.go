package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
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

func countEvents(t *testing.T, ctx context.Context) int {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var count int
	require.NoError(t, exec.QueryRowContext(ctx,
		`SELECT count(*) FROM domain_events WHERE entity_type_id = 1600`).Scan(&count))
	return count
}
