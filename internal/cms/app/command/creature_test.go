package command_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/command"
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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
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

var cardNameSuffixes sync.Map

// 開発用の DB に残っているカードの名前とぶつからないよう、テストごとに別の名前にする（同じテストの中では同じ値）
func cardName(t *testing.T, base string) string {
	t.Helper()
	suffix, _ := cardNameSuffixes.LoadOrStore(t.Name(), uuid.NewString())
	return fmt.Sprintf("%s-%s", base, suffix)
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
	ctx := signedIn(t, rdbtest.Tx(t), registrant(t))
	name := cardName(t, "ドラゴン")

	registered, err := newCreatureCommands(t, ctx).Register(ctx, "  "+name+" ")

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, registered.ID)
	require.Equal(t, name, registered.Name)
}

func TestRegisterCreature_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, rdbtest.Tx(t), registrant(t))
	commands := newCreatureCommands(t, ctx)
	_, err := commands.Register(ctx, "ＡＢＣ"+cardName(t, ""))
	require.NoError(t, err)

	_, err = commands.Register(ctx, " ABC"+cardName(t, ""))

	require.ErrorIs(t, err, command.ErrCardNameAlreadyInUse)
}

func TestRegisterCreature_RejectsInvalidName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  command.InputError
	}{
		{name: "blank", value: " ", want: &command.RequiredInput{Field: "name"}},
		{name: "too_long", value: strings.Repeat("あ", card.MaxNameLength+1), want: &command.TooLongInput{Field: "name", Max: card.MaxNameLength}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, rdbtest.Tx(t), registrant(t))

			_, err := newCreatureCommands(t, ctx).Register(ctx, tt.value)

			invalid, ok := command.AsInputErrors(err)
			require.True(t, ok)
			require.Equal(t, command.InputErrors{tt.want}, invalid)
		})
	}
}

func TestRegisterCreature_RejectsWithoutOperator(t *testing.T) {
	ctx := rdbtest.Tx(t)

	_, err := newCreatureCommands(t, ctx).Register(ctx, cardName(t, "ドラゴン"))

	require.ErrorIs(t, err, command.ErrNotOperator)
}

func TestRegisterCreature_RecordsOperatorInEvent(t *testing.T) {
	by := operatortest.ID(t, 42)
	ctx := signedIn(t, rdbtest.Tx(t), by)
	name := cardName(t, "ドラゴン")
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
	db := rdbtest.Open(t)
	name := cardName(t, "同時")
	constrainttest.Cleanup(t, db, card.NameKey(cardtest.Name(t, name)))
	ctx := signedIn(t, rdb.WithDB(t.Context(), db), registrant(t))
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
