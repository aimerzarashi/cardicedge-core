package command_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

func newCommands(t *testing.T, ctx context.Context) *command.OperatorCommands {
	t.Helper()
	return newCommandsWithExists(t, ctx, constraint.OperatorExists)
}

// オペレーターがいるかを引く関数を差し替える。開発用の DB には初期オペレーターがいて、別のパッケージのテストも
// 同時にオペレーターを commit するため、「1 人もいない」状態を DB で作ると、時間しだいで結果が変わる
func newCommandsWithExists(t *testing.T, ctx context.Context, exists operator.ExistsFunc) *command.OperatorCommands {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewOperatorCommands(
		eventstore.New(registry, operator.NewID, operator.NewEventType, operator.InitialState, operator.Evolve),
		identifier.New(registry, operator.NewID),
		constraint.UpdateOperatorEmail,
		constraint.UpdateOperatorSubject,
		constraint.FindOperatorBySubject,
		constraint.FindUnlinkedOperatorByEmail,
		exists,
		func() time.Time { return time.Now().UTC() },
	)
}

// 登録して、そのオペレーターの ID を返す（紐づける前なので、メールアドレスで引ける）
func register(t *testing.T, ctx context.Context, commands *command.OperatorCommands, email string) operator.ID {
	t.Helper()
	require.NoError(t, commands.Register(ctx, email))
	id, found, err := constraint.FindUnlinkedOperatorByEmail(ctx, operatortest.Email(t, email))
	require.NoError(t, err)
	require.True(t, found)
	return id
}

// cms の JWT（iss は diecardomain-cms）で認証し、context に載ったオペレーターを返す
func authenticate(t *testing.T, ctx context.Context, commands *command.OperatorCommands, subject, email string, emailVerified bool) (operator.ID, bool) {
	t.Helper()
	authenticated, err := commands.Authenticate(ctx, "diecardomain-cms", subject, email, emailVerified)
	require.NoError(t, err)
	return authentication.Operator(authenticated)
}

var emailSuffixes sync.Map

// 開発用の DB には起動時に登録した初期オペレーターがいるため、テストごとに別のメールアドレスにする（同じテストの中では同じ値）
func email(t *testing.T, local string) string {
	t.Helper()
	suffix, _ := emailSuffixes.LoadOrStore(t.Name(), uuid.NewString())
	return fmt.Sprintf("%s-%s@example.com", local, suffix)
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
	ctx := rdbtest.Tx(t)
	commands := newCommands(t, ctx)

	id := register(t, ctx, commands, email(t, "operator"))

	require.Equal(t, 1, countEvents(t, ctx, id))
}

func TestRegister_RejectsEmailAlreadyRegistered(t *testing.T) {
	ctx := rdbtest.Tx(t)
	commands := newCommands(t, ctx)
	register(t, ctx, commands, email(t, "operator"))

	err := commands.Register(ctx, email(t, "operator"))

	require.ErrorIs(t, err, command.ErrOperatorEmailAlreadyRegistered)
}

func TestRegister_RejectsInvalidEmail(t *testing.T) {
	ctx := rdbtest.Tx(t)

	err := newCommands(t, ctx).Register(ctx, "not-an-email")

	invalid, ok := command.AsInputErrors(err)
	require.True(t, ok)
	require.Equal(t, command.InputErrors{&command.InvalidFormatInput{Field: "email"}}, invalid)
}

func TestRegisterInitial_RegistersOnlyWhenNoOperatorExists(t *testing.T) {
	tests := []struct {
		name           string
		exists         bool
		wantRegistered bool
	}{
		{name: "no_operator", exists: false, wantRegistered: true},
		{name: "operator_exists", exists: true, wantRegistered: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := rdbtest.Tx(t)
			commands := newCommandsWithExists(t, ctx, func(context.Context) (bool, error) { return tt.exists, nil })

			registered, err := commands.RegisterInitial(ctx, email(t, "initial"))

			require.NoError(t, err)
			require.Equal(t, tt.wantRegistered, registered)
			_, found, err := constraint.FindUnlinkedOperatorByEmail(ctx, operatortest.Email(t, email(t, "initial")))
			require.NoError(t, err)
			require.Equal(t, tt.wantRegistered, found)
		})
	}
}

func TestAuthenticate_LinksOperatorByVerifiedEmailOnFirstCall(t *testing.T) {
	ctx := rdbtest.Tx(t)
	commands := newCommands(t, ctx)
	registered := register(t, ctx, commands, email(t, "operator"))

	first, found := authenticate(t, ctx, commands, "user-1", email(t, "operator"), true)
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
		email         string // メールアドレスのローカル部（テストの中で email で一意にする）。空ならメールアドレス無し
		emailVerified bool
		setup         func(t *testing.T, ctx context.Context, commands *command.OperatorCommands)
	}{
		{name: "without_email", setup: func(t *testing.T, ctx context.Context, commands *command.OperatorCommands) {
			register(t, ctx, commands, email(t, "operator"))
		}},
		{name: "unverified_email", email: "operator", emailVerified: false, setup: func(t *testing.T, ctx context.Context, commands *command.OperatorCommands) {
			register(t, ctx, commands, email(t, "operator"))
		}},
		{name: "unregistered_email", email: "other", emailVerified: true, setup: func(t *testing.T, ctx context.Context, commands *command.OperatorCommands) {
			register(t, ctx, commands, email(t, "operator"))
		}},
		{name: "operator_linked_to_other_subject", email: "operator", emailVerified: true, setup: func(t *testing.T, ctx context.Context, commands *command.OperatorCommands) {
			register(t, ctx, commands, email(t, "operator"))
			_, found := authenticate(t, ctx, commands, "user-2", email(t, "operator"), true)
			require.True(t, found)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := rdbtest.Tx(t)
			commands := newCommands(t, ctx)
			tt.setup(t, ctx, commands)
			address := ""
			if tt.email != "" {
				address = email(t, tt.email)
			}

			_, found := authenticate(t, ctx, commands, "user-1", address, tt.emailVerified)

			require.False(t, found)
		})
	}
}

func TestAuthenticate_RejectsInvalidCredentials(t *testing.T) {
	ctx := rdbtest.Tx(t)

	_, err := newCommands(t, ctx).Authenticate(ctx, "diecardomain-cms", " ", "", false)

	require.ErrorIs(t, err, command.ErrInvalidCredentials)
}

// cms が初めてのログインの直後に複数の画面から同時に呼んでも、紐づけは 1 回だけ
func TestAuthenticate_LinksOnceWhenCalledConcurrently(t *testing.T) {
	db := rdbtest.Open(t)
	email := fmt.Sprintf("concurrent-%s@example.com", uuid.NewString())
	constrainttest.Cleanup(t, db, operator.EmailKey(operatortest.Email(t, email)))
	ctx := rdb.WithDB(t.Context(), db)
	commands := newCommands(t, ctx)
	registered := register(t, ctx, commands, email)

	const callers = 20
	ids := make([]operator.ID, callers)
	founds := make([]bool, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			var authenticated context.Context
			authenticated, errs[i] = commands.Authenticate(ctx, "diecardomain-cms", email, email, true)
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
