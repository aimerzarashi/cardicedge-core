package command_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell/spelltest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/input"
)

func newSpellHandler(t *testing.T, ctx context.Context) *command.SpellHandler {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewSpellHandler(
		eventstore.New(registry, spell.NewID, spell.NewEventType, spell.InitialState, spell.Evolve),
		identifier.New(registry, spell.NewID),
		constraint.UpdateSpellName,
		func() time.Time { return time.Now().UTC() },
	)
}

func countSpellEvents(t *testing.T, ctx context.Context, id spell.ID) int {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var count int
	require.NoError(t, exec.QueryRowContext(ctx,
		`SELECT count(*) FROM domain_events WHERE entity_type_id = 4000 AND entity_id = $1`, id.Int64()).Scan(&count))
	return count
}

// app 層は uif に内部 ID を返さないため、カード名の key から引く
func spellIDByName(t *testing.T, ctx context.Context, name string) spell.ID {
	t.Helper()
	return spelltest.ID(t, constrainttest.Owner(t, ctx, card.NameKey(cardtest.Name(t, name))))
}

func TestRegisterSpell_ReturnsRegisteredSpell(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	name := "ファイアボール"
	before := time.Now().UTC()

	registered, err := newSpellHandler(t, ctx).Register(ctx, "  "+name+" ")

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, registered.ID)
	require.Equal(t, name, registered.Name)
	require.WithinRange(t, registered.RegisteredAt, before, time.Now().UTC())
	// 登録した直後は変更が無い
	require.Equal(t, registered.RegisteredAt, registered.UpdatedAt)
}

func TestRegisterSpell_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newSpellHandler(t, ctx)
	_, err := commands.Register(ctx, "ＡＢＣ")
	require.NoError(t, err)

	_, err = commands.Register(ctx, " ABC")

	require.ErrorIs(t, err, command.ErrCardNameAlreadyInUse)
}

// カード名は Creature・Item と共有しているため、種類をまたいでも重複になる
func TestRegisterSpell_RejectsNameUsedByOtherCardType(t *testing.T) {
	tests := []struct {
		name     string
		register func(t *testing.T, ctx context.Context, name string) error
	}{
		{
			name: "creature",
			register: func(t *testing.T, ctx context.Context, name string) error {
				_, err := newCreatureHandler(t, ctx).Register(ctx, name)
				return err
			},
		},
		{
			name: "item",
			register: func(t *testing.T, ctx context.Context, name string) error {
				_, err := newItemHandler(t, ctx).Register(ctx, name)
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			require.NoError(t, tt.register(t, ctx, "ドラゴン"))

			_, err := newSpellHandler(t, ctx).Register(ctx, " ドラゴン")

			require.ErrorIs(t, err, command.ErrCardNameAlreadyInUse)
		})
	}
}

func TestRegisterSpell_RejectsInvalidName(t *testing.T) {
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

			_, err := newSpellHandler(t, ctx).Register(ctx, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
		})
	}
}

func TestRegisterSpell_RejectsWithoutOperator(t *testing.T) {
	ctx := isolatedDatabase(t)

	_, err := newSpellHandler(t, ctx).Register(ctx, "ファイアボール")

	require.ErrorIs(t, err, command.ErrNotOperator)
}

func TestRegisterSpell_RecordsOperatorInEvent(t *testing.T) {
	by := operatortest.ID(t, 42)
	ctx := signedIn(t, isolatedDatabase(t), by)
	name := "ファイアボール"
	_, err := newSpellHandler(t, ctx).Register(ctx, name)
	require.NoError(t, err)

	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id := spellIDByName(t, ctx, name)
	restored, err := eventstore.New(registry, spell.NewID, spell.NewEventType, spell.InitialState, spell.Evolve).Load(ctx, id)
	require.NoError(t, err)
	require.Equal(t, by, restored.(spell.Spell).RegisteredBy)
	require.Equal(t, 1, countSpellEvents(t, ctx, id))
}

// 同時に同じ名前で登録しても、1 つだけ登録される
func TestRegisterSpell_RegistersOnceWhenCalledConcurrently(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	name := "ファイアボール"
	commands := newSpellHandler(t, ctx)

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
	spellIDByName(t, ctx, name)
}
