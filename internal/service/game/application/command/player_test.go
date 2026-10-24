package command_test

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
)

func TestRegister_RegistersPlayerOnFirstCall(t *testing.T) {
	ctx := isolatedDatabase(t)
	commands := newPlayerHandler(t, ctx)

	registered, err := commands.Register(authenticate(t, ctx, commands, "user-1"))

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, registered.ID)
	require.Equal(t, 1, countEvents(t, ctx))
}

func TestRegister_ReturnsSamePlayerWithoutNewEventWhenCalledAgain(t *testing.T) {
	ctx := isolatedDatabase(t)
	commands := newPlayerHandler(t, ctx)
	first, err := commands.Register(authenticate(t, ctx, commands, "user-1"))
	require.NoError(t, err)

	// 登録した後の Authenticate はプレイヤーも載せるが、Register はどちらの context でも同じ結果を返す
	second, err := commands.Register(authenticate(t, ctx, commands, "user-1"))

	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, 1, countEvents(t, ctx))
}

func TestRegister_RegistersOtherPlayerForOtherSubject(t *testing.T) {
	ctx := isolatedDatabase(t)
	commands := newPlayerHandler(t, ctx)
	first, err := commands.Register(authenticate(t, ctx, commands, "user-1"))
	require.NoError(t, err)

	second, err := commands.Register(authenticate(t, ctx, commands, "user-2"))

	require.NoError(t, err)
	require.NotEqual(t, first, second)
	require.Equal(t, 2, countEvents(t, ctx))
}

func TestRegister_RejectsContextWithoutSubject(t *testing.T) {
	ctx := isolatedDatabase(t)

	_, err := newPlayerHandler(t, ctx).Register(ctx)

	require.ErrorIs(t, err, command.ErrNotAuthenticated)
}

// game が初めてのログインの直後に複数の画面から同時に呼んでも、プレイヤーは 1 人だけ
func TestRegister_RegistersOnceWhenCalledConcurrently(t *testing.T) {
	ctx := isolatedDatabase(t)
	commands := newPlayerHandler(t, ctx)
	authenticated := authenticate(t, ctx, commands, "user-1")

	const callers = 20
	registered := make([]command.RegisteredPlayer, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			registered[i], errs[i] = commands.Register(authenticated)
		})
	}
	wg.Wait()

	for i := range callers {
		require.NoError(t, errs[i])
		require.Equal(t, registered[0], registered[i])
	}
	require.Equal(t, 1, countEvents(t, ctx))
}

func TestMe_ReturnsRegisteredPlayer(t *testing.T) {
	ctx := isolatedDatabase(t)
	commands := newPlayerHandler(t, ctx)
	registered, err := commands.Register(authenticate(t, ctx, commands, "user-1"))
	require.NoError(t, err)

	me, err := commands.Me(authenticate(t, ctx, commands, "user-1"))

	require.NoError(t, err)
	require.Equal(t, registered, me)
}

func TestMe_RejectsPlayerNotRegistered(t *testing.T) {
	ctx := isolatedDatabase(t)
	commands := newPlayerHandler(t, ctx)

	_, err := commands.Me(authenticate(t, ctx, commands, "user-1"))

	require.ErrorIs(t, err, command.ErrPlayerNotRegistered)
}

func TestAuthenticate_RejectsInvalidCredentials(t *testing.T) {
	tests := []struct {
		name    string
		issuer  string
		subject string
	}{
		{name: "blank_issuer", issuer: " ", subject: "user-1"},
		{name: "blank_subject", issuer: "diecardomain-game", subject: " "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := isolatedDatabase(t)

			_, err := newPlayerHandler(t, ctx).Authenticate(ctx, tt.issuer, tt.subject)

			require.ErrorIs(t, err, command.ErrInvalidCredentials)
		})
	}
}
