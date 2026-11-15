package effect_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect/effecttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/element"
)

func TestNewEffects_RejectsAnEffectWithoutAKind(t *testing.T) {
	_, err := effect.NewEffects([]effect.Effect{{}})

	require.ErrorIs(t, err, effect.ErrEffectEmpty)
}

func TestNewKind_AcceptsTheKindsOfTheEffects(t *testing.T) {
	for _, value := range []effect.Effect{
		effecttest.AttackModifier(1), effecttest.HitPointsModifier(1), effecttest.AttackOverride(t, 30), effecttest.HitPointsOverrideRange(t, 10, 70),
		effecttest.AttackMultiplier(t, 150), effect.NewIgnoreTerrainEffect(), effect.NewStrikeFirstEffect(), effect.NewStrikeLastEffect(),
		effecttest.NullifyReceivedDamage(t, "fire"), effecttest.TollMultiplier(t, 150), effect.NewForbidInvasionEffect(), effect.NewForbidMovementEffect(),
	} {
		kind, err := effect.NewKind(value.Kind().String())

		require.NoError(t, err)
		require.Equal(t, value.Kind(), kind)
	}
	_, err := effect.NewKind("defensive")
	require.ErrorIs(t, err, effect.ErrKindUnknown)
}

func TestNewAttackOverride_RejectsNegative(t *testing.T) {
	_, err := effect.NewAttackOverride(-1)

	require.ErrorIs(t, err, effect.ErrAttackOverrideNegative)
}

// HP 0 は戦闘の始めに倒れるため 1 以上（core#178）
func TestNewHitPointsOverride_RejectsBelowOne(t *testing.T) {
	_, err := effect.NewHitPointsOverride(0)

	require.ErrorIs(t, err, effect.ErrHitPointsOverrideTooSmall)
}

// 固定の上書きは最小と最大が同じ範囲
func TestNewOverrideEffects_AcceptEqualBoundsAndRejectMaximumBelowMinimum(t *testing.T) {
	low, err := effect.NewAttackOverride(30)
	require.NoError(t, err)
	high, err := effect.NewAttackOverride(29)
	require.NoError(t, err)
	_, equalErr := effect.NewAttackOverrideEffect(low, low)
	_, attackErr := effect.NewAttackOverrideEffect(low, high)
	lowHitPoints, err := effect.NewHitPointsOverride(30)
	require.NoError(t, err)
	highHitPoints, err := effect.NewHitPointsOverride(29)
	require.NoError(t, err)
	_, hitPointsErr := effect.NewHitPointsOverrideEffect(lowHitPoints, highHitPoints)

	require.NoError(t, equalErr)
	require.ErrorIs(t, attackErr, effect.ErrOverrideMaximumLessThanMinimum)
	require.ErrorIs(t, hitPointsErr, effect.ErrOverrideMaximumLessThanMinimum)
}

func TestNewPercents_RejectBelowOne(t *testing.T) {
	_, attackErr := effect.NewAttackMultiplierPercent(0)
	_, tollErr := effect.NewTollMultiplierPercent(0)

	require.ErrorIs(t, attackErr, effect.ErrAttackMultiplierPercentTooSmall)
	require.ErrorIs(t, tollErr, effect.ErrTollMultiplierPercentTooSmall)
}

// 制約の属性は無とほかを一緒に選べる（クリーチャー自身の属性の「無は無だけ」は当てない。core#178）
func TestNewCreatureElements_SortsAcceptsNeutralWithOthersAndRejectsInvalidValues(t *testing.T) {
	elements, err := effect.NewCreatureElements([]string{"neutral", "water", "fire"})
	require.NoError(t, err)
	require.Equal(t, effect.CreatureElements{element.CreatureFire, element.CreatureWater, element.CreatureNeutral}, elements)

	_, emptyErr := effect.NewCreatureElements(nil)
	_, unknownErr := effect.NewCreatureElements([]string{"multi"})
	_, duplicatedErr := effect.NewCreatureElements([]string{"fire", "fire"})
	require.ErrorIs(t, emptyErr, effect.ErrCreatureElementsEmpty)
	require.ErrorIs(t, unknownErr, effect.ErrCreatureElementUnknown)
	require.ErrorIs(t, duplicatedErr, effect.ErrCreatureElementsDuplicated)
}

func TestConstraints_AreSatisfiedWhenAnyOpponentCreatureElementMatchesEveryConstraint(t *testing.T) {
	strike := effecttest.AttackMultiplier(t, 150, "water", "wind").AttackMultiplier.Constraints

	require.True(t, strike.SatisfiedByOpponentCreatureElements([]element.Creature{element.CreatureFire, element.CreatureWind}))
	require.False(t, strike.SatisfiedByOpponentCreatureElements([]element.Creature{element.CreatureFire, element.CreatureEarth}))
	require.True(t, effect.Constraints{}.SatisfiedByOpponentCreatureElements([]element.Creature{element.CreatureFire}))
}

func TestEffects_AreComparedByValueAndFoundByKind(t *testing.T) {
	spectre := effecttest.Effects(t, effecttest.AttackOverrideRange(t, 10, 70), effecttest.HitPointsOverrideRange(t, 10, 70))

	require.True(t, spectre.Equal(effecttest.Effects(t, effecttest.AttackOverrideRange(t, 10, 70), effecttest.HitPointsOverrideRange(t, 10, 70))))
	require.False(t, spectre.Equal(effecttest.Effects(t, effecttest.AttackOverrideRange(t, 10, 60), effecttest.HitPointsOverrideRange(t, 10, 70))))
	require.False(t, effecttest.Effects(t, effecttest.NullifyReceivedDamage(t, "fire")).Equal(effecttest.Effects(t, effecttest.NullifyReceivedDamage(t, "water"))))
	require.True(t, spectre.Has(effect.KindHitPointsOverride))
	require.False(t, spectre.Has(effect.KindStrikeFirst))
}
