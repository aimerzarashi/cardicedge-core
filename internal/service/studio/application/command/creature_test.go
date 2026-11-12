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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature/creaturetest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

func countCreatureEvents(t *testing.T, ctx context.Context, id creature.ID) int {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var count int
	require.NoError(t, exec.QueryRowContext(ctx,
		`SELECT count(*) FROM domain_events WHERE entity_type_id = 1100 AND entity_id = $1`, id.Int64()).Scan(&count))
	return count
}

// application 層は presentation 層に内部 ID を返さないため、カード名の key から引く
func creatureIDByName(t *testing.T, ctx context.Context, name string) creature.ID {
	t.Helper()
	return creaturetest.ID(t, constrainttest.Owner(t, ctx, card.NameKey(cardtest.Name(t, name))))
}

func TestRegisterCreature_ReturnsRegisteredCreature(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	name := "ドラゴン"
	before := time.Now().UTC()

	registered, err := newCreatureHandler(t, ctx).Register(ctx, "  "+name+" ")

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, registered.ID)
	require.Equal(t, name, registered.Name)
	require.WithinRange(t, registered.RegisteredAt, before, time.Now().UTC())
	// 登録した直後は変更が無い
	require.Equal(t, registered.RegisteredAt, registered.UpdatedAt)
}

func TestRegisterCreature_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newCreatureHandler(t, ctx)
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

			_, err := newCreatureHandler(t, ctx).Register(ctx, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
		})
	}
}

func TestRegisterCreature_RejectsWithoutOperator(t *testing.T) {
	ctx := isolatedDatabase(t)

	_, err := newCreatureHandler(t, ctx).Register(ctx, "ドラゴン")

	require.ErrorIs(t, err, command.ErrNotOperator)
}

func TestRegisterCreature_RecordsOperatorInEvent(t *testing.T) {
	by := operatortest.ID(t, 42)
	ctx := signedIn(t, isolatedDatabase(t), by)
	name := "ドラゴン"
	_, err := newCreatureHandler(t, ctx).Register(ctx, name)
	require.NoError(t, err)

	id := creatureIDByName(t, ctx, name)
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var registeredBy int64
	require.NoError(t, exec.QueryRowContext(ctx,
		`SELECT (payload->>'registered_by')::bigint FROM domain_events WHERE entity_type_id = 1100 AND entity_id = $1`, id.Int64()).Scan(&registeredBy))
	require.Equal(t, by.Int64(), registeredBy)
	require.Equal(t, 1, countCreatureEvents(t, ctx, id))
}

// 同時に同じ名前で登録しても、1 つだけ登録される
func TestRegisterCreature_RegistersOnceWhenCalledConcurrently(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	name := "ドラゴン"
	commands := newCreatureHandler(t, ctx)

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

func TestRegisterCreature_ReturnsDefaultSpec(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))

	registered, err := newCreatureHandler(t, ctx).Register(ctx, "ドラゴン")

	require.NoError(t, err)
	require.Equal(t, []string{"neutral"}, registered.Elements)
	require.Equal(t, creature.DefaultAttack.Int(), registered.Attack)
	require.Equal(t, creature.DefaultHitPoints.Int(), registered.HitPoints)
	require.Empty(t, registered.Abilities)
}

func TestChangeCreatureAbilities_ChangesDraftAndRejectsDuplicates(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newCreatureHandler(t, ctx)
	registered, err := commands.Register(ctx, "ドラゴン")
	require.NoError(t, err)
	defensive := command.CreatureAbility{Kind: command.CreatureAbilityDefensive}

	changed, err := commands.ChangeAbilities(ctx, registered.ID, registered.Revision, []command.CreatureAbility{defensive})
	require.NoError(t, err)
	require.Equal(t, command.ChangedCard{ID: registered.ID, Revision: registered.Revision + 1}, changed)
	unchanged, err := commands.ChangeAbilities(ctx, changed.ID, changed.Revision, []command.CreatureAbility{defensive})
	require.NoError(t, err)
	require.Equal(t, changed, unchanged)
	_, err = commands.ChangeAbilities(ctx, changed.ID, changed.Revision, []command.CreatureAbility{defensive, defensive})
	require.Equal(t, input.Errors{&input.InvalidFormat{Field: "abilities"}}, err)
}

func TestChangeCreatureItemRestrictions_ChangesDraftAndRejectsUnknown(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newCreatureHandler(t, ctx)
	registered, err := commands.Register(ctx, "ドラゴン")
	require.NoError(t, err)
	require.Empty(t, registered.ItemRestrictions)

	changed, err := commands.ChangeItemRestrictions(ctx, registered.ID, registered.Revision, []string{"scroll", "armor"})
	require.NoError(t, err)
	require.Equal(t, command.ChangedCard{ID: registered.ID, Revision: registered.Revision + 1}, changed)
	unchanged, err := commands.ChangeItemRestrictions(ctx, changed.ID, changed.Revision, []string{"armor", "scroll"})
	require.NoError(t, err)
	require.Equal(t, changed, unchanged)
	_, err = commands.ChangeItemRestrictions(ctx, changed.ID, changed.Revision, []string{"tool"})
	require.Equal(t, input.Errors{&input.InvalidFormat{Field: "itemRestrictions"}}, err)
}

func TestChangeCreaturePlacementRestrictions_ChangesDraftAndRejectsNeutral(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newCreatureHandler(t, ctx)
	registered, err := commands.Register(ctx, "ドラゴン")
	require.NoError(t, err)
	require.Empty(t, registered.PlacementRestrictions)

	changed, err := commands.ChangePlacementRestrictions(ctx, registered.ID, registered.Revision, []string{"wind", "fire"})
	require.NoError(t, err)
	require.Equal(t, command.ChangedCard{ID: registered.ID, Revision: registered.Revision + 1}, changed)
	unchanged, err := commands.ChangePlacementRestrictions(ctx, changed.ID, changed.Revision, []string{"fire", "wind"})
	require.NoError(t, err)
	require.Equal(t, changed, unchanged)
	_, err = commands.ChangePlacementRestrictions(ctx, changed.ID, changed.Revision, []string{"neutral"})
	require.Equal(t, input.Errors{&input.InvalidFormat{Field: "placementRestrictions"}}, err)
}

func TestChangeCreatureAbilities_RejectsMismatchedValuesAndTooSmallPercent(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newCreatureHandler(t, ctx)
	registered, err := commands.Register(ctx, "ドラゴン")
	require.NoError(t, err)
	zero, half := 0, 150

	changed, err := commands.ChangeAbilities(ctx, registered.ID, registered.Revision, []command.CreatureAbility{{Kind: command.CreatureAbilityTollMultiplier, Percent: &half}})
	require.NoError(t, err)
	require.Equal(t, registered.Revision+1, changed.Revision)
	_, err = commands.ChangeAbilities(ctx, changed.ID, changed.Revision, []command.CreatureAbility{
		{Kind: command.CreatureAbilityDefensive, Percent: &half},
		{Kind: command.CreatureAbilityTollMultiplier, Percent: &zero},
		{Kind: command.CreatureAbilityTollMultiplier},
	})
	require.Equal(t, input.Errors{
		&input.InvalidFormat{Field: "abilities.0"},
		&input.TooSmall{Field: "abilities.1.percent", Min: 1},
		&input.InvalidFormat{Field: "abilities.2"},
	}, err)
}

func TestChangeCreatureAbilities_BattleModifierNeedsBothValuesAndOtherKindsHaveNone(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newCreatureHandler(t, ctx)
	registered, err := commands.Register(ctx, "ドラゴン")
	require.NoError(t, err)
	negative, zero, percent := -10, 0, 150

	changed, err := commands.ChangeAbilities(ctx, registered.ID, registered.Revision, []command.CreatureAbility{{Kind: command.CreatureAbilityBattleModifier, AttackModifier: &negative, HitPointsModifier: &zero}})
	require.NoError(t, err)
	require.Equal(t, registered.Revision+1, changed.Revision)
	_, err = commands.ChangeAbilities(ctx, changed.ID, changed.Revision, []command.CreatureAbility{
		{Kind: command.CreatureAbilityBattleModifier, AttackModifier: &negative},
		{Kind: command.CreatureAbilityBattleModifier, HitPointsModifier: &zero},
		{Kind: command.CreatureAbilityBattleModifier, AttackModifier: &negative, HitPointsModifier: &zero, Percent: &percent},
		{Kind: command.CreatureAbilityDefensive, AttackModifier: &negative},
		{Kind: command.CreatureAbilityTollMultiplier, Percent: &percent, HitPointsModifier: &zero},
	})
	require.Equal(t, input.Errors{
		&input.InvalidFormat{Field: "abilities.0"},
		&input.InvalidFormat{Field: "abilities.1"},
		&input.InvalidFormat{Field: "abilities.2"},
		&input.InvalidFormat{Field: "abilities.3"},
		&input.InvalidFormat{Field: "abilities.4"},
	}, err)
}

func TestChangeCreatureAbilities_BattleRandomStatsNeedsBothBoundsAndOtherKindsHaveNone(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newCreatureHandler(t, ctx)
	registered, err := commands.Register(ctx, "ドラゴン")
	require.NoError(t, err)
	minimum, maximum, percent, modifier := 10, 70, 150, 0

	changed, err := commands.ChangeAbilities(ctx, registered.ID, registered.Revision, []command.CreatureAbility{{Kind: command.CreatureAbilityBattleRandomStats, Minimum: &minimum, Maximum: &maximum}})
	require.NoError(t, err)
	require.Equal(t, registered.Revision+1, changed.Revision)
	_, err = commands.ChangeAbilities(ctx, changed.ID, changed.Revision, []command.CreatureAbility{
		{Kind: command.CreatureAbilityBattleRandomStats, Minimum: &minimum},
		{Kind: command.CreatureAbilityBattleRandomStats, Maximum: &maximum},
		{Kind: command.CreatureAbilityBattleRandomStats, Minimum: &minimum, Maximum: &maximum, Percent: &percent},
		{Kind: command.CreatureAbilityBattleRandomStats, Minimum: &minimum, Maximum: &maximum, AttackModifier: &modifier},
		{Kind: command.CreatureAbilityDefensive, Minimum: &minimum},
		{Kind: command.CreatureAbilityBattleModifier, AttackModifier: &modifier, HitPointsModifier: &modifier, Maximum: &maximum},
	})
	require.Equal(t, input.Errors{
		&input.InvalidFormat{Field: "abilities.0"},
		&input.InvalidFormat{Field: "abilities.1"},
		&input.InvalidFormat{Field: "abilities.2"},
		&input.InvalidFormat{Field: "abilities.3"},
		&input.InvalidFormat{Field: "abilities.4"},
		&input.InvalidFormat{Field: "abilities.5"},
	}, err)
}

func TestChangeCreatureAbilities_RejectsBattleRandomStatsRangeOutOfOrder(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newCreatureHandler(t, ctx)
	registered, err := commands.Register(ctx, "ドラゴン")
	require.NoError(t, err)
	zero, ten, nine := 0, 10, 9

	_, err = commands.ChangeAbilities(ctx, registered.ID, registered.Revision, []command.CreatureAbility{
		{Kind: command.CreatureAbilityBattleRandomStats, Minimum: &zero, Maximum: &ten},
		{Kind: command.CreatureAbilityBattleRandomStats, Minimum: &ten, Maximum: &nine},
	})

	require.Equal(t, input.Errors{
		&input.TooSmall{Field: "abilities.0.minimum", Min: 1},
		&input.TooSmall{Field: "abilities.1.maximum", Min: 10},
	}, err)
}

func TestChangeCreatureAbilities_RejectsFirstStrikeWithLastStrike(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newCreatureHandler(t, ctx)
	registered, err := commands.Register(ctx, "ドラゴン")
	require.NoError(t, err)
	percent := 150

	changed, err := commands.ChangeAbilities(ctx, registered.ID, registered.Revision, []command.CreatureAbility{{Kind: command.CreatureAbilityFirstStrike}})
	require.NoError(t, err)
	require.Equal(t, registered.Revision+1, changed.Revision)
	_, err = commands.ChangeAbilities(ctx, changed.ID, changed.Revision, []command.CreatureAbility{{Kind: command.CreatureAbilityFirstStrike}, {Kind: command.CreatureAbilityLastStrike}})
	require.Equal(t, input.Errors{&input.InvalidFormat{Field: "abilities"}}, err)
	_, err = commands.ChangeAbilities(ctx, changed.ID, changed.Revision, []command.CreatureAbility{{Kind: command.CreatureAbilityLastStrike, Percent: &percent}})
	require.Equal(t, input.Errors{&input.InvalidFormat{Field: "abilities.0"}}, err)
}

func TestChangeCreatureElementsAndStats_ChangeSpecOfDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newCreatureHandler(t, ctx)
	registered, err := commands.Register(ctx, "ドラゴン")
	require.NoError(t, err)

	elementsChanged, err := commands.ChangeElements(ctx, registered.ID, registered.Revision, []string{"wind", "fire"})
	require.NoError(t, err)
	statsChanged, err := commands.ChangeStats(ctx, elementsChanged.ID, elementsChanged.Revision, 0, 60)

	require.NoError(t, err)
	require.Equal(t, command.ChangedCard{ID: registered.ID, Revision: registered.Revision + 1}, elementsChanged)
	require.Equal(t, command.ChangedCard{ID: registered.ID, Revision: registered.Revision + 2}, statsChanged)
}

// 同じ値への変更は、イベントを作らず revision を進めない
func TestChangeCreatureElementsAndStats_KeepRevisionWhenUnchanged(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newCreatureHandler(t, ctx)
	registered, err := commands.Register(ctx, "ドラゴン")
	require.NoError(t, err)

	elementsChanged, elementsErr := commands.ChangeElements(ctx, registered.ID, registered.Revision, []string{"neutral"})
	statsChanged, statsErr := commands.ChangeStats(ctx, registered.ID, registered.Revision, creature.DefaultAttack.Int(), creature.DefaultHitPoints.Int())

	require.NoError(t, elementsErr)
	require.NoError(t, statsErr)
	require.Equal(t, command.ChangedCard{ID: registered.ID, Revision: registered.Revision}, elementsChanged)
	require.Equal(t, command.ChangedCard{ID: registered.ID, Revision: registered.Revision}, statsChanged)
}

func TestChangeCreatureElementsAndStats_RejectInvalidValue(t *testing.T) {
	tests := []struct {
		name   string
		change func(ctx context.Context, commands *command.CreatureHandler, registered command.RegisteredCreature) error
		want   []input.Error
	}{
		{name: "elements_empty", change: func(ctx context.Context, c *command.CreatureHandler, r command.RegisteredCreature) error {
			_, err := c.ChangeElements(ctx, r.ID, r.Revision, nil)
			return err
		}, want: []input.Error{&input.Required{Field: "elements"}}},
		{name: "elements_unknown", change: func(ctx context.Context, c *command.CreatureHandler, r command.RegisteredCreature) error {
			_, err := c.ChangeElements(ctx, r.ID, r.Revision, []string{"light"})
			return err
		}, want: []input.Error{&input.InvalidFormat{Field: "elements"}}},
		{name: "elements_neutral_mixed", change: func(ctx context.Context, c *command.CreatureHandler, r command.RegisteredCreature) error {
			_, err := c.ChangeElements(ctx, r.ID, r.Revision, []string{"neutral", "fire"})
			return err
		}, want: []input.Error{&input.InvalidFormat{Field: "elements"}}},
		{name: "elements_duplicated", change: func(ctx context.Context, c *command.CreatureHandler, r command.RegisteredCreature) error {
			_, err := c.ChangeElements(ctx, r.ID, r.Revision, []string{"fire", "fire"})
			return err
		}, want: []input.Error{&input.InvalidFormat{Field: "elements"}}},
		{name: "attack_negative", change: func(ctx context.Context, c *command.CreatureHandler, r command.RegisteredCreature) error {
			_, err := c.ChangeStats(ctx, r.ID, r.Revision, -1, 10)
			return err
		}, want: []input.Error{&input.TooSmall{Field: "attack", Min: 0}}},
		{name: "hit_points_zero", change: func(ctx context.Context, c *command.CreatureHandler, r command.RegisteredCreature) error {
			_, err := c.ChangeStats(ctx, r.ID, r.Revision, 10, 0)
			return err
		}, want: []input.Error{&input.TooSmall{Field: "hitPoints", Min: 1}}},
		{name: "attack_and_hit_points", change: func(ctx context.Context, c *command.CreatureHandler, r command.RegisteredCreature) error {
			_, err := c.ChangeStats(ctx, r.ID, r.Revision, -1, 0)
			return err
		}, want: []input.Error{&input.TooSmall{Field: "attack", Min: 0}, &input.TooSmall{Field: "hitPoints", Min: 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			commands := newCreatureHandler(t, ctx)
			registered, err := commands.Register(ctx, "ドラゴン")
			require.NoError(t, err)

			err = tt.change(ctx, commands, registered)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
		})
	}
}

func TestChangeCreatureElementsAndStats_RejectNotDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newCreatureHandler(t, ctx)
	registered, err := commands.Register(ctx, "ドラゴン")
	require.NoError(t, err)
	published, err := commands.Publish(ctx, registered.ID, registered.Revision)
	require.NoError(t, err)

	_, elementsErr := commands.ChangeElements(ctx, published.ID, published.Revision, []string{"fire"})
	_, statsErr := commands.ChangeStats(ctx, published.ID, published.Revision, 30, 50)
	_, abilitiesErr := commands.ChangeAbilities(ctx, published.ID, published.Revision, []command.CreatureAbility{{Kind: command.CreatureAbilityDefensive}})

	require.ErrorIs(t, elementsErr, command.ErrCardNotDraft)
	require.ErrorIs(t, statsErr, command.ErrCardNotDraft)
	require.ErrorIs(t, abilitiesErr, command.ErrCardNotDraft)
}

func TestChangeCreatureElementsAndStats_RejectStaleRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newCreatureHandler(t, ctx)
	stale, err := commands.Register(ctx, "ドラゴン")
	require.NoError(t, err)
	_, err = commands.ChangeStats(ctx, stale.ID, stale.Revision, 30, 50)
	require.NoError(t, err)

	_, elementsErr := commands.ChangeElements(ctx, stale.ID, stale.Revision, []string{"fire"})
	_, statsErr := commands.ChangeStats(ctx, stale.ID, stale.Revision, 40, 60)

	require.ErrorIs(t, elementsErr, command.ErrRevisionConflict)
	require.ErrorIs(t, statsErr, command.ErrRevisionConflict)
}

func TestChangeCreatureElementsAndStats_RejectWithoutOperator(t *testing.T) {
	ctx := isolatedDatabase(t)
	commands := newCreatureHandler(t, ctx)

	_, elementsErr := commands.ChangeElements(ctx, uuid.New(), 1, []string{"fire"})
	_, statsErr := commands.ChangeStats(ctx, uuid.New(), 1, 30, 50)

	require.ErrorIs(t, elementsErr, command.ErrNotOperator)
	require.ErrorIs(t, statsErr, command.ErrNotOperator)
}
