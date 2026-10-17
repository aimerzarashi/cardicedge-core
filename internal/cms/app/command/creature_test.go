package command_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature/creaturetest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

func newCreatureCommands(t *testing.T, ctx context.Context) *command.CreatureCommands {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewCreatureCommands(
		eventstore.New(registry, creature.NewID, creature.NewEventType, creature.InitialState, creature.Evolve),
		identifier.New(registry, creature.NewID),
		constraint.UpdateCreatureName,
		func() time.Time { return time.Now().UTC() },
	)
}

// 認証ミドルウェアが載せるのと同じく、操作した人を context に載せる。
// イベントに内部 ID を残すだけなので、オペレーターは実在しなくてよい
func signedIn(t *testing.T, ctx context.Context, by operator.ID) context.Context {
	t.Helper()
	return authentication.WithOperator(ctx, by)
}

func registrant(t *testing.T) operator.ID {
	t.Helper()
	return operatortest.ID(t, 10)
}

func countCreatureEvents(t *testing.T, ctx context.Context, id creature.ID) int {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var count int
	require.NoError(t, exec.QueryRowContext(ctx,
		`SELECT count(*) FROM domain_events WHERE entity_type_id = 2000 AND entity_id = $1`, id.Int64()).Scan(&count))
	return count
}

// app 層は uif に内部 ID を返さないため、カード名の key から引く
func creatureIDByName(t *testing.T, ctx context.Context, name string) creature.ID {
	t.Helper()
	return creaturetest.ID(t, constrainttest.Owner(t, ctx, card.NameKey(cardtest.Name(t, name))))
}

func TestRegisterCreature_ReturnsRegisteredCreature(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	name := "ドラゴン"
	before := time.Now().UTC()

	registered, err := newCreatureCommands(t, ctx).Register(ctx, "  "+name+" ")

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, registered.ID)
	require.Equal(t, name, registered.Name)
	require.WithinRange(t, registered.RegisteredAt, before, time.Now().UTC())
	// 登録した直後は変更が無い
	require.Equal(t, registered.RegisteredAt, registered.UpdatedAt)
}

func TestRegisterCreature_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newCreatureCommands(t, ctx)
	_, err := commands.Register(ctx, "ＡＢＣ")
	require.NoError(t, err)

	_, err = commands.Register(ctx, " ABC")

	require.ErrorIs(t, err, command.ErrCardNameAlreadyInUse)
}

func TestRegisterCreature_RejectsInvalidName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  input.Error
	}{
		{name: "blank", value: " ", want: &input.Required{Field: "name"}},
		{name: "too_long", value: strings.Repeat("あ", card.MaxNameLength+1), want: &input.TooLong{Field: "name", Max: card.MaxNameLength}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))

			_, err := newCreatureCommands(t, ctx).Register(ctx, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
		})
	}
}

func TestRegisterCreature_RejectsWithoutOperator(t *testing.T) {
	ctx := isolatedDatabase(t)

	_, err := newCreatureCommands(t, ctx).Register(ctx, "ドラゴン")

	require.ErrorIs(t, err, command.ErrNotOperator)
}

func TestRegisterCreature_RecordsOperatorInEvent(t *testing.T) {
	by := operatortest.ID(t, 42)
	ctx := signedIn(t, isolatedDatabase(t), by)
	name := "ドラゴン"
	_, err := newCreatureCommands(t, ctx).Register(ctx, name)
	require.NoError(t, err)

	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id := creatureIDByName(t, ctx, name)
	restored, err := eventstore.New(registry, creature.NewID, creature.NewEventType, creature.InitialState, creature.Evolve).Load(ctx, id)
	require.NoError(t, err)
	require.Equal(t, by, restored.(creature.Creature).RegisteredBy)
	require.Equal(t, 1, countCreatureEvents(t, ctx, id))
}

// 同時に同じ名前で登録しても、1 つだけ登録される
func TestRegisterCreature_RegistersOnceWhenCalledConcurrently(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	name := "ドラゴン"
	commands := newCreatureCommands(t, ctx)

	const callers = 20
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			_, errs[i] = commands.Register(ctx, name)
		})
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		require.ErrorIs(t, err, command.ErrCardNameAlreadyInUse)
	}
	require.Equal(t, 1, succeeded)
	creatureIDByName(t, ctx, name)
}
