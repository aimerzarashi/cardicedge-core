package effecttest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 生成関数のエラーを毎回確かめると冗長なため、ここで確かめる

func Effects(t testing.TB, values ...effect.Effect) effect.Effects {
	t.Helper()
	effects, err := effect.NewEffects(values)
	require.NoError(t, err)
	return effects
}

func AttackModifier(value int) effect.Effect {
	return effect.NewAttackModifierEffect(effect.NewAttackModifier(value))
}

func HitPointsModifier(value int) effect.Effect {
	return effect.NewHitPointsModifierEffect(effect.NewHitPointsModifier(value))
}

// 固定の AT の上書き（最小と最大が同じ）
func AttackOverride(t testing.TB, value int) effect.Effect {
	t.Helper()
	return AttackOverrideRange(t, value, value)
}

func AttackOverrideRange(t testing.TB, minimum, maximum int) effect.Effect {
	t.Helper()
	low, err := effect.NewAttackOverride(minimum)
	require.NoError(t, err)
	high, err := effect.NewAttackOverride(maximum)
	require.NoError(t, err)
	override, err := effect.NewAttackOverrideEffect(low, high)
	require.NoError(t, err)
	return override
}

func HitPointsOverrideRange(t testing.TB, minimum, maximum int) effect.Effect {
	t.Helper()
	low, err := effect.NewHitPointsOverride(minimum)
	require.NoError(t, err)
	high, err := effect.NewHitPointsOverride(maximum)
	require.NoError(t, err)
	override, err := effect.NewHitPointsOverrideEffect(low, high)
	require.NoError(t, err)
	return override
}

// 相手のクリーチャーの属性を制約にした AT の倍率（強打）。属性を渡さなければ制約なし
func AttackMultiplier(t testing.TB, percent int, opponentElements ...string) effect.Effect {
	t.Helper()
	multiplier, err := effect.NewAttackMultiplierPercent(percent)
	require.NoError(t, err)
	return effect.NewAttackMultiplierEffect(multiplier, opponentCreatureElements(t, opponentElements))
}

// 相手のクリーチャーの属性を制約にした、受けるダメージを 0 にする（攻撃無効）
func NullifyReceivedDamage(t testing.TB, opponentElements ...string) effect.Effect {
	t.Helper()
	return effect.NewNullifyReceivedDamageEffect(opponentCreatureElements(t, opponentElements))
}

func TollMultiplier(t testing.TB, percent int) effect.Effect {
	t.Helper()
	value, err := effect.NewTollMultiplierPercent(percent)
	require.NoError(t, err)
	return effect.NewTollMultiplierEffect(value)
}

func opponentCreatureElements(t testing.TB, values []string) effect.Constraints {
	t.Helper()
	var constraints []effect.Constraint
	if len(values) > 0 {
		elements, err := effect.NewCreatureElements(values)
		require.NoError(t, err)
		constraints = append(constraints, effect.NewOpponentCreatureElementConstraint(elements))
	}
	built, err := effect.NewConstraints(constraints)
	require.NoError(t, err)
	return built
}

func Destructions(t testing.TB, values ...effect.Destruction) effect.Destructions {
	t.Helper()
	raw := make([]string, len(values))
	for i, value := range values {
		raw[i] = string(value)
	}
	destructions, err := effect.NewDestructions(raw)
	require.NoError(t, err)
	return destructions
}

func ReturnToHandOnDestruction(t testing.TB, destructions ...effect.Destruction) effect.Effect {
	t.Helper()
	return effect.NewReturnToHandOnDestructionEffect(Destructions(t, destructions...))
}

func GainMagicOnDestruction(t testing.TB, amount int, destructions ...effect.Destruction) effect.Effect {
	t.Helper()
	value, err := effect.NewDestructionMagicAmount(amount)
	require.NoError(t, err)
	return effect.NewGainMagicOnDestructionEffect(Destructions(t, destructions...), value)
}
