package command_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar/avatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/rules"
)

const avatarTypeID = 2200

func reservedAvatars(t *testing.T, ctx context.Context) int {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var used int
	require.NoError(t, exec.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(q.used), 0) FROM domain_quotas q JOIN domain_quota_types t ON t.id = q.quota_type_id
		WHERE t.name = $1`, avatar.PlayerQuotaType.String()).Scan(&used))
	return used
}

func TestRegisterAvatar_CreatesAvatarOfPlayerWithName(t *testing.T) {
	ctx := isolatedDatabase(t)
	player := registeredPlayer(t, ctx, "user-1")

	registered, err := newAvatarHandlerWithMax(t, ctx, 5).Register(player, " アリス ")

	require.NoError(t, err)
	require.Equal(t, "アリス", registered.Name)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := identifier.New(registry, avatar.NewID).Resolve(ctx, registered.ID)
	require.NoError(t, err)
	state, err := eventstore.New(registry, avatar.NewID, avatar.NewEventType, avatar.InitialState, avatar.Evolve).Load(ctx, id)
	require.NoError(t, err)
	created, ok := state.(avatar.Avatar)
	require.True(t, ok)
	require.Equal(t, avatartest.Name(t, "アリス"), created.Name)
	require.Equal(t, registered.ID, created.PublicID)
	require.Equal(t, 1, countEventsOf(t, ctx, avatarTypeID))
	require.Equal(t, 1, reservedAvatars(t, ctx))
}

func TestRegisterAvatar_RejectsInvalidName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  input.Error
	}{
		{name: "blank", value: "  ", want: &input.Required{Field: "name"}},
		{name: "too_long", value: strings.Repeat("あ", avatar.MaxNameLength+1), want: &input.TooLong{Field: "name", Max: avatar.MaxNameLength}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := isolatedDatabase(t)
			player := registeredPlayer(t, ctx, "user-1")

			_, err := newAvatarHandlerWithMax(t, ctx, 5).Register(player, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
			require.Zero(t, countEventsOf(t, ctx, avatarTypeID))
			require.Zero(t, reservedAvatars(t, ctx))
		})
	}
}

func TestRegisterAvatar_RejectsSameNameOfSamePlayer(t *testing.T) {
	ctx := isolatedDatabase(t)
	player := registeredPlayer(t, ctx, "user-1")
	handler := newAvatarHandlerWithMax(t, ctx, 5)
	_, err := handler.Register(player, "アリス")
	require.NoError(t, err)

	_, err = handler.Register(player, " アリス ")

	require.ErrorIs(t, err, command.ErrAvatarNameAlreadyInUse)
	require.Equal(t, 1, countEventsOf(t, ctx, avatarTypeID))
	require.Equal(t, 1, reservedAvatars(t, ctx))
}

func TestRegisterAvatar_AllowsSameNameOfOtherPlayer(t *testing.T) {
	ctx := isolatedDatabase(t)
	first := registeredPlayer(t, ctx, "user-1")
	second := registeredPlayer(t, ctx, "user-2")
	handler := newAvatarHandlerWithMax(t, ctx, 5)
	_, err := handler.Register(first, "アリス")
	require.NoError(t, err)

	_, err = handler.Register(second, "アリス")

	require.NoError(t, err)
}

func TestRegisterAvatar_RejectsAvatarOverLimitWithMax(t *testing.T) {
	ctx := isolatedDatabase(t)
	player := registeredPlayer(t, ctx, "user-1")
	handler := newAvatarHandlerWithMax(t, ctx, 2)
	for _, name := range []string{"一", "二"} {
		_, err := handler.Register(player, name)
		require.NoError(t, err)
	}

	_, err := handler.Register(player, "三")

	require.ErrorIs(t, err, command.ErrAvatarLimitReached)
	var limitReached *command.LimitReachedError
	require.ErrorAs(t, err, &limitReached)
	require.Equal(t, 2, limitReached.Max)
	require.Equal(t, 2, countEventsOf(t, ctx, avatarTypeID))
	require.Equal(t, 2, reservedAvatars(t, ctx))
}

// 別のプレイヤーのアバターの数は、上限に数えない
func TestRegisterAvatar_CountsLimitPerPlayer(t *testing.T) {
	ctx := isolatedDatabase(t)
	first := registeredPlayer(t, ctx, "user-1")
	second := registeredPlayer(t, ctx, "user-2")
	handler := newAvatarHandlerWithMax(t, ctx, 1)
	_, err := handler.Register(first, "一")
	require.NoError(t, err)

	_, err = handler.Register(second, "一")

	require.NoError(t, err)
}

func TestRegisterAvatar_DoesNotExceedLimitWhenCalledConcurrently(t *testing.T) {
	ctx := isolatedDatabase(t)
	player := registeredPlayer(t, ctx, "user-1")
	handler := newAvatarHandlerWithMax(t, ctx, 3)

	const callers = 12
	var succeeded, reached atomic.Int32
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			_, err := handler.Register(player, string(rune('あ'+i)))
			switch {
			case err == nil:
				succeeded.Add(1)
			case errors.Is(err, command.ErrAvatarLimitReached):
				reached.Add(1)
			}
		})
	}
	wg.Wait()

	require.Equal(t, int32(3), succeeded.Load())
	require.Equal(t, int32(callers-3), reached.Load())
	require.Equal(t, 3, countEventsOf(t, ctx, avatarTypeID))
	require.Equal(t, 3, reservedAvatars(t, ctx))
}

func TestRegisterAvatar_RejectsPlayerNotRegistered(t *testing.T) {
	ctx := isolatedDatabase(t)
	players := newPlayerHandler(t, ctx)
	unregistered := authenticate(t, ctx, players, "user-1")

	_, err := newAvatarHandlerWithMax(t, ctx, 5).Register(unregistered, "アリス")

	require.ErrorIs(t, err, command.ErrPlayerNotRegistered)
	require.Zero(t, countEventsOf(t, ctx, avatarTypeID))
}

// 写されていないルールの代わりに既定の値で作らない。何も残さない
func TestRegisterAvatar_RejectsWhenRulesNotPublished(t *testing.T) {
	ctx := isolatedDatabase(t)
	player := registeredPlayer(t, ctx, "user-1")
	handler := newAvatarHandlerWithRules(t, ctx, func(context.Context) (gamerules.Rules, bool, error) {
		return gamerules.Rules{}, false, nil
	})

	_, err := handler.Register(player, "アリス")

	require.ErrorIs(t, err, rules.ErrNotPublished)
	require.Zero(t, countEventsOf(t, ctx, avatarTypeID))
	require.Zero(t, reservedAvatars(t, ctx))
}
