package command_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item/itemtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

func countItemEvents(t *testing.T, ctx context.Context, id item.ID) int {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var count int
	require.NoError(t, exec.QueryRowContext(ctx,
		`SELECT count(*) FROM domain_events WHERE entity_type_id = 1200 AND entity_id = $1`, id.Int64()).Scan(&count))
	return count
}

// application 層は presentation 層に内部 ID を返さないため、カード名の key から引く
func itemIDByName(t *testing.T, ctx context.Context, name string) item.ID {
	t.Helper()
	return itemtest.ID(t, constrainttest.Owner(t, ctx, card.NameKey(cardtest.Name(t, name))))
}

func TestRegisterItem_ReturnsRegisteredItem(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	name := "ポーション"
	before := time.Now().UTC()

	registered, err := newItemHandler(t, ctx).Register(ctx, "  "+name+" ")

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, registered.ID)
	require.Equal(t, name, registered.Name)
	require.WithinRange(t, registered.RegisteredAt, before, time.Now().UTC())
	// 登録した直後は変更が無い
	require.Equal(t, registered.RegisteredAt, registered.UpdatedAt)
}

func TestRegisterItem_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newItemHandler(t, ctx)
	_, err := commands.Register(ctx, "ＡＢＣ")
	require.NoError(t, err)

	_, err = commands.Register(ctx, " ABC")

	require.ErrorIs(t, err, command.ErrCardNameAlreadyInUse)
}

// カード名は Creature と共有しているため、種類をまたいでも重複になる
func TestRegisterItem_RejectsNameUsedByCreature(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	_, err := newCreatureHandler(t, ctx).Register(ctx, "ドラゴン")
	require.NoError(t, err)

	_, err = newItemHandler(t, ctx).Register(ctx, " ドラゴン")

	require.ErrorIs(t, err, command.ErrCardNameAlreadyInUse)
}

func TestRegisterItem_RejectsInvalidName(t *testing.T) {
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

			_, err := newItemHandler(t, ctx).Register(ctx, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
		})
	}
}

func TestRegisterItem_RejectsWithoutOperator(t *testing.T) {
	ctx := isolatedDatabase(t)

	_, err := newItemHandler(t, ctx).Register(ctx, "ポーション")

	require.ErrorIs(t, err, command.ErrNotOperator)
}

func TestRegisterItem_RecordsOperatorInEvent(t *testing.T) {
	by := operatortest.ID(t, 42)
	ctx := signedIn(t, isolatedDatabase(t), by)
	name := "ポーション"
	_, err := newItemHandler(t, ctx).Register(ctx, name)
	require.NoError(t, err)

	id := itemIDByName(t, ctx, name)
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var registeredBy int64
	require.NoError(t, exec.QueryRowContext(ctx,
		`SELECT (payload->>'registered_by')::bigint FROM domain_events WHERE entity_type_id = 1200 AND entity_id = $1`, id.Int64()).Scan(&registeredBy))
	require.Equal(t, by.Int64(), registeredBy)
	require.Equal(t, 1, countItemEvents(t, ctx, id))
}

// 同時に同じ名前で登録しても、1 つだけ登録される
func TestRegisterItem_RegistersOnceWhenCalledConcurrently(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	name := "ポーション"
	commands := newItemHandler(t, ctx)

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
	itemIDByName(t, ctx, name)
}

func TestChangeItemCategoryAndEffects_ChangeSpecOfDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newItemHandler(t, ctx)
	registered, err := commands.Register(ctx, "まきもの")
	require.NoError(t, err)

	categorized, err := commands.ChangeCategory(ctx, registered.ID, registered.Revision, "scroll")
	require.NoError(t, err)
	changed, err := commands.ChangeEffects(ctx, categorized.ID, categorized.Revision, []command.Effect{
		{Kind: command.EffectAttackOverride, Minimum: new(30), Maximum: new(30)},
		{Kind: command.EffectIgnoreTerrainEffect},
		{Kind: command.EffectAttackMultiplier, Percent: new(150), Constraints: []command.EffectConstraint{{Kind: command.EffectConstraintOpponentCreatureElement, Elements: []string{"fire"}}}},
	})

	require.NoError(t, err)
	require.Equal(t, registered.Revision+2, changed.Revision)
}

func TestChangeItemEffects_RejectsInvalidInputAtEachField(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newItemHandler(t, ctx)
	registered, err := commands.Register(ctx, "まきもの")
	require.NoError(t, err)

	_, err = commands.ChangeEffects(ctx, registered.ID, registered.Revision, []command.Effect{
		{Kind: command.EffectAttackModifier},
		{Kind: command.EffectAttackOverride, Minimum: new(-1), Maximum: new(30)},
		{Kind: command.EffectAttackMultiplier, Percent: new(0), Constraints: []command.EffectConstraint{{Kind: command.EffectConstraintOpponentCreatureElement, Elements: []string{"multi"}}, {Kind: "unknown"}}},
	})

	require.Equal(t, input.Errors{
		&input.InvalidFormat{Field: "effects.0"},
		&input.TooSmall{Field: "effects.1.minimum", Min: 0},
		&input.TooSmall{Field: "effects.2.percent", Min: 1},
		&input.InvalidFormat{Field: "effects.2.constraints.0.elements"},
		&input.InvalidFormat{Field: "effects.2.constraints.1"},
	}, err)
}

func TestChangeItemEffectsAndCategory_RejectEffectsTheCategoryDoesNotAllow(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newItemHandler(t, ctx)
	registered, err := commands.Register(ctx, "ぶき")
	require.NoError(t, err)
	changed, err := commands.ChangeEffects(ctx, registered.ID, registered.Revision, []command.Effect{{Kind: command.EffectAttackModifier, Value: new(10)}})
	require.NoError(t, err)

	_, effectErr := commands.ChangeEffects(ctx, changed.ID, changed.Revision, []command.Effect{{Kind: command.EffectHitPointsModifier, Value: new(10)}})
	_, categoryErr := commands.ChangeCategory(ctx, changed.ID, changed.Revision, "armor")

	require.ErrorIs(t, effectErr, command.ErrItemEffectNotAllowed)
	require.ErrorIs(t, categoryErr, command.ErrItemEffectNotAllowed)
}

func TestChangeItemCategory_RejectsUnknownValue(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newItemHandler(t, ctx)
	registered, err := commands.Register(ctx, "かぶと")
	require.NoError(t, err)

	_, err = commands.ChangeCategory(ctx, registered.ID, registered.Revision, "shield")

	require.Equal(t, input.Errors{&input.InvalidFormat{Field: "category"}}, err)
}

func TestChangeItemEffects_RejectsPublishedItem(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newItemHandler(t, ctx)
	registered, err := commands.Register(ctx, "かぶと")
	require.NoError(t, err)
	published, err := commands.Publish(ctx, registered.ID, registered.Revision)
	require.NoError(t, err)

	_, err = commands.ChangeEffects(ctx, published.ID, published.Revision, []command.Effect{{Kind: command.EffectAttackModifier, Value: new(5)}})

	require.ErrorIs(t, err, command.ErrCardNotDraft)
}
