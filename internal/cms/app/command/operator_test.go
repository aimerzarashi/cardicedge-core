package command_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/typeregistry"
)

func newCommands(t *testing.T, ctx context.Context) *command.OperatorCommands {
	t.Helper()
	registry, err := typeregistry.Load(ctx)
	require.NoError(t, err)
	return command.NewOperatorCommands(
		eventstore.New(registry, operator.NewID, operator.NewEventType, operator.InitialState, operator.Evolve),
		identifier.New(registry, operator.NewID),
		constraint.NewOperators(),
		func() time.Time { return time.Now().UTC() },
	)
}

func register(t *testing.T, ctx context.Context, commands *command.OperatorCommands, email string) operator.ID {
	t.Helper()
	id, err := commands.Register(ctx, operatortest.Email(t, email))
	require.NoError(t, err)
	return id
}

func identify(t *testing.T, ctx context.Context, commands *command.OperatorCommands, subject string, verifiedEmail *operator.Email) (operator.ID, bool) {
	t.Helper()
	id, found, err := commands.Identify(ctx, operatortest.Issuer(t, "diecardomain-cms"), operatortest.Subject(t, subject), verifiedEmail)
	require.NoError(t, err)
	return id, found
}

func emailPtr(t *testing.T, value string) *operator.Email {
	t.Helper()
	email := operatortest.Email(t, value)
	return &email
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

// commit するテストで作った行を消す（domain_identifiers を消すと operators・operator_subjects も CASCADE で消える）
func cleanup(t *testing.T, db *sql.DB, email string) {
	t.Cleanup(func() {
		ctx := context.WithoutCancel(t.Context())
		_, _ = db.ExecContext(ctx, `DELETE FROM domain_events WHERE entity_type_id = 1000 AND entity_id IN (SELECT id FROM operators WHERE email = $1)`, email)
		_, _ = db.ExecContext(ctx, `DELETE FROM domain_identifiers WHERE id IN (SELECT id FROM operators WHERE email = $1)`, email)
	})
}

func TestRegister_RegistersOperatorWithEmail(t *testing.T) {
	ctx := rdbtest.Tx(t)
	commands := newCommands(t, ctx)

	id := register(t, ctx, commands, email(t, "operator"))

	found, ok, err := constraint.NewOperators().FindUnlinkedByEmail(ctx, operatortest.Email(t, email(t, "operator")))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, id, found)
	require.Equal(t, 1, countEvents(t, ctx, id))
}

func TestRegister_RejectsEmailAlreadyRegistered(t *testing.T) {
	ctx := rdbtest.Tx(t)
	commands := newCommands(t, ctx)
	register(t, ctx, commands, email(t, "operator"))

	_, err := commands.Register(ctx, operatortest.Email(t, email(t, "operator")))

	require.ErrorIs(t, err, operator.ErrAlreadyRegistered)
}

func TestRegisterInitial_RegistersOnlyWhenNoOperatorExists(t *testing.T) {
	ctx := rdbtest.Tx(t)
	// 開発用の DB には、起動時に登録した初期オペレーターがいる。このトランザクションの中だけで消す（最後に rollback する）
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	_, err = exec.ExecContext(ctx, `DELETE FROM domain_identifiers WHERE id IN (SELECT id FROM operators)`)
	require.NoError(t, err)
	commands := newCommands(t, ctx)

	first, err := commands.RegisterInitial(ctx, operatortest.Email(t, email(t, "initial")))
	require.NoError(t, err)
	second, err := commands.RegisterInitial(ctx, operatortest.Email(t, email(t, "changed")))
	require.NoError(t, err)

	require.True(t, first)
	require.False(t, second)
	_, found, err := constraint.NewOperators().FindUnlinkedByEmail(ctx, operatortest.Email(t, email(t, "changed")))
	require.NoError(t, err)
	require.False(t, found)
}

func TestIdentify_LinksOperatorByVerifiedEmailOnFirstCall(t *testing.T) {
	ctx := rdbtest.Tx(t)
	commands := newCommands(t, ctx)
	registered := register(t, ctx, commands, email(t, "operator"))

	first, found := identify(t, ctx, commands, "user-1", emailPtr(t, email(t, "operator")))
	require.True(t, found)
	// 紐づけた後は、メールアドレスが無くても (iss, sub) で引ける
	second, found := identify(t, ctx, commands, "user-1", nil)
	require.True(t, found)

	require.Equal(t, registered, first)
	require.Equal(t, registered, second)
	require.Equal(t, 2, countEvents(t, ctx, registered))
}

func TestIdentify_ReturnsNotFoundWhenNotOperator(t *testing.T) {
	tests := []struct {
		name          string
		verifiedEmail string // メールアドレスのローカル部（テストの中で email で一意にする）。空なら確認済みのメールアドレス無し
		setup         func(t *testing.T, ctx context.Context, commands *command.OperatorCommands)
	}{
		{name: "without_verified_email", verifiedEmail: "", setup: func(t *testing.T, ctx context.Context, commands *command.OperatorCommands) {
			register(t, ctx, commands, email(t, "operator"))
		}},
		{name: "unregistered_email", verifiedEmail: "other", setup: func(t *testing.T, ctx context.Context, commands *command.OperatorCommands) {
			register(t, ctx, commands, email(t, "operator"))
		}},
		{name: "operator_linked_to_other_subject", verifiedEmail: "operator", setup: func(t *testing.T, ctx context.Context, commands *command.OperatorCommands) {
			register(t, ctx, commands, email(t, "operator"))
			_, found := identify(t, ctx, commands, "user-2", emailPtr(t, email(t, "operator")))
			require.True(t, found)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := rdbtest.Tx(t)
			commands := newCommands(t, ctx)
			tt.setup(t, ctx, commands)
			var verifiedEmail *operator.Email
			if tt.verifiedEmail != "" {
				verifiedEmail = emailPtr(t, email(t, tt.verifiedEmail))
			}

			_, found := identify(t, ctx, commands, "user-1", verifiedEmail)

			require.False(t, found)
		})
	}
}

// cms が初めてのログインの直後に複数の画面から同時に呼んでも、紐づけは 1 回だけ
func TestIdentify_LinksOnceWhenCalledConcurrently(t *testing.T) {
	db := rdbtest.Open(t)
	email := fmt.Sprintf("concurrent-%s@example.com", uuid.NewString())
	cleanup(t, db, email)
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
			ids[i], founds[i], errs[i] = commands.Identify(ctx, operatortest.Issuer(t, "diecardomain-cms"), operatortest.Subject(t, email), emailPtr(t, email))
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
