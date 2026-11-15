package itemtest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 生成関数のエラーを毎回確かめると冗長なため、ここで確かめる

func ID(t testing.TB, value int64) item.ID {
	t.Helper()
	id, err := item.NewID(value)
	require.NoError(t, err)
	return id
}

func Effects(t testing.TB, values ...item.Effect) item.Effects {
	t.Helper()
	effects, err := item.NewEffects(values)
	require.NoError(t, err)
	return effects
}

func AttackOverrideEffect(t testing.TB, value int) item.Effect {
	t.Helper()
	override, err := item.NewAttackOverride(value)
	require.NoError(t, err)
	return item.NewAttackOverrideEffect(override)
}

// 相手の属性を制約にした AT の倍率（強打）
func AttackMultiplierEffect(t testing.TB, percent int, opponentElements ...string) item.Effect {
	t.Helper()
	multiplier, err := item.NewAttackMultiplierPercent(percent)
	require.NoError(t, err)
	var constraints []item.Constraint
	if len(opponentElements) > 0 {
		elements, err := item.NewElements(opponentElements)
		require.NoError(t, err)
		constraints = append(constraints, item.NewOpponentElementConstraint(elements))
	}
	built, err := item.NewConstraints(constraints)
	require.NoError(t, err)
	return item.NewAttackMultiplierEffect(multiplier, built)
}
