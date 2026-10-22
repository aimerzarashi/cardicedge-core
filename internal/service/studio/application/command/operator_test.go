package command_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

func newOperatorHandler(t *testing.T, ctx context.Context) *command.OperatorHandler {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewOperatorHandler(
		eventstore.New(registry, operator.NewID, operator.NewEventType, operator.InitialState, operator.Evolve),
		identifier.New(registry, operator.NewID),
		constraint.UpdateOperatorEmail,
		constraint.UpdateOperatorSubject,
		constraint.FindOperatorBySubject,
		constraint.FindUnlinkedOperatorByEmail,
		constraint.OperatorExists,
		func() time.Time { return time.Now().UTC() },
	)
}

// 登録して、そのオペレーターの ID を返す（紐づける前なので、メールアドレスで引ける）
func register(t *testing.T, ctx context.Context, commands *command.OperatorHandler, email string) operator.ID {
	t.Helper()
	require.NoError(t, commands.Register(ctx, email))
	id, found, err := constraint.FindUnlinkedOperatorByEmail(ctx, operatortest.Email(t, email))
	require.NoError(t, err)
	require.True(t, found)
	return id
}

// studio の JWT（iss は diecardomain-studio）で認証し、context に載ったオペレーターを返す
func authenticate(t *testing.T, ctx context.Context, commands *command.OperatorHandler, subject, email string, emailVerified bool) (operator.ID, bool) {
	t.Helper()
	authenticated, err := commands.Authenticate(ctx, "diecardomain-studio", subject, email, emailVerified)
	require.NoError(t, err)
	return authentication.Operator(authenticated)
}

func countEvents(t *testing.T, ctx context.Context, id operator.ID) int {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var count int
	require.NoError(t, exec.QueryRowContext(ctx,
		`SELECT count(*) FROM domain_events WHERE entity_type_id = 1000 AND entity_id = $1`, id.Int64()).Scan(&count))
	return count
}

func TestRegister_RegistersOperatorWithEmail(t *testing.T) {
	ctx := isolatedDatabase(t)
	commands := newOperatorHandler(t, ctx)

	id := register(t, ctx, commands, "operator@example.com")

	require.Equal(t, 1, countEvents(t, ctx, id))
}

func TestRegister_RejectsEmailAlreadyRegistered(t *testing.T) {
	ctx := isolatedDatabase(t)
	commands := newOperatorHandler(t, ctx)
	register(t, ctx, commands, "operator@example.com")

	err := commands.Register(ctx, "operator@example.com")

	require.ErrorIs(t, err, command.ErrOperatorEmailAlreadyRegistered)
}

func TestRegister_RejectsInvalidEmail(t *testing.T) {
	ctx := isolatedDatabase(t)

	err := newOperatorHandler(t, ctx).Register(ctx, "not-an-email")

	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, input.Errors{&input.InvalidFormat{Field: "email"}}, invalid)
}

func TestRegisterInitial_RegistersOnlyWhenNoOperatorExists(t *testing.T) {
	tests := []struct {
		name           string
		existing       string // 先に登録しておくオペレーターのメールアドレス。空なら登録しない
		wantRegistered bool
	}{
		{name: "no_operator", wantRegistered: true},
		{name: "operator_exists", existing: "operator@example.com", wantRegistered: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := isolatedDatabase(t)
			commands := newOperatorHandler(t, ctx)
			if tt.existing != "" {
				register(t, ctx, commands, tt.existing)
			}

			registered, err := commands.RegisterInitial(ctx, "initial@example.com")

			require.NoError(t, err)
			require.Equal(t, tt.wantRegistered, registered)
			_, found, err := constraint.FindUnlinkedOperatorByEmail(ctx, operatortest.Email(t, "initial@example.com"))
			require.NoError(t, err)
			require.Equal(t, tt.wantRegistered, found)
		})
	}
}

func TestAuthenticate_LinksOperatorByVerifiedEmailOnFirstCall(t *testing.T) {
	ctx := isolatedDatabase(t)
	commands := newOperatorHandler(t, ctx)
	registered := register(t, ctx, commands, "operator@example.com")

	first, found := authenticate(t, ctx, commands, "user-1", "operator@example.com", true)
	require.True(t, found)
	// 紐づけた後は、メールアドレスが無くても (iss, sub) で引ける
	second, found := authenticate(t, ctx, commands, "user-1", "", false)
	require.True(t, found)

	require.Equal(t, registered, first)
	require.Equal(t, registered, second)
	require.Equal(t, 2, countEvents(t, ctx, registered))
}

func TestAuthenticate_DoesNotSetOperatorWhenNotOperator(t *testing.T) {
	tests := []struct {
		name          string
		email         string // 空ならメールアドレス無し
		emailVerified bool
		setup         func(t *testing.T, ctx context.Context, commands *command.OperatorHandler)
	}{
		{name: "without_email", setup: func(t *testing.T, ctx context.Context, commands *command.OperatorHandler) {
			register(t, ctx, commands, "operator@example.com")
		}},
		{name: "unverified_email", email: "operator@example.com", emailVerified: false, setup: func(t *testing.T, ctx context.Context, commands *command.OperatorHandler) {
			register(t, ctx, commands, "operator@example.com")
		}},
		{name: "unregistered_email", email: "other@example.com", emailVerified: true, setup: func(t *testing.T, ctx context.Context, commands *command.OperatorHandler) {
			register(t, ctx, commands, "operator@example.com")
		}},
		{name: "operator_linked_to_other_subject", email: "operator@example.com", emailVerified: true, setup: func(t *testing.T, ctx context.Context, commands *command.OperatorHandler) {
			register(t, ctx, commands, "operator@example.com")
			_, found := authenticate(t, ctx, commands, "user-2", "operator@example.com", true)
			require.True(t, found)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := isolatedDatabase(t)
			commands := newOperatorHandler(t, ctx)
			tt.setup(t, ctx, commands)
			_, found := authenticate(t, ctx, commands, "user-1", tt.email, tt.emailVerified)

			require.False(t, found)
		})
	}
}

func TestAuthenticate_RejectsInvalidCredentials(t *testing.T) {
	ctx := isolatedDatabase(t)

	_, err := newOperatorHandler(t, ctx).Authenticate(ctx, "diecardomain-studio", " ", "", false)

	require.ErrorIs(t, err, command.ErrInvalidCredentials)
}

// studio が初めてのログインの直後に複数の画面から同時に呼んでも、紐づけは 1 回だけ
func TestAuthenticate_LinksOnceWhenCalledConcurrently(t *testing.T) {
	ctx := isolatedDatabase(t)
	email := "operator@example.com"
	commands := newOperatorHandler(t, ctx)
	registered := register(t, ctx, commands, email)

	const callers = 20
	ids := make([]operator.ID, callers)
	founds := make([]bool, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			var authenticated context.Context
			authenticated, errs[i] = commands.Authenticate(ctx, "diecardomain-studio", email, email, true)
			if errs[i] == nil {
				ids[i], founds[i] = authentication.Operator(authenticated)
			}
		})
	}
	wg.Wait()

	for i := range callers {
		require.NoError(t, errs[i])
		require.True(t, founds[i])
		require.Equal(t, registered, ids[i])
	}
	require.Equal(t, 2, countEvents(t, ctx, registered))
}
