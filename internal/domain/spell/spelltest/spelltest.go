package spelltest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 生成関数のエラーを毎回確かめると冗長なため、ここで確かめる

func ID(t testing.TB, value int64) spell.ID {
	t.Helper()
	id, err := spell.NewID(value)
	require.NoError(t, err)
	return id
}

// カードを引く効果
func DrawCards(t testing.TB, count int) spell.Effect {
	t.Helper()
	drawCount, err := spell.NewDrawCount(count)
	require.NoError(t, err)
	return spell.NewDrawCardsSpellEffect(drawCount)
}

// 魔力を得る効果
func GainMagic(t testing.TB, perLap int) spell.Effect {
	t.Helper()
	value, err := spell.NewMagicPerLap(perLap)
	require.NoError(t, err)
	return spell.NewGainMagicSpellEffect(value)
}

// サイコロを複数振る効果
func MultipleDice(t testing.TB, count, turns int) spell.Effect {
	t.Helper()
	value, err := spell.NewDiceCount(count)
	require.NoError(t, err)
	return spell.NewMultipleDiceSpellEffect(value, effectTurns(t, turns))
}

// サイコロの目を固定する効果
func FixedDice(t testing.TB, value, turns int) spell.Effect {
	t.Helper()
	face, err := spell.NewDiceFace(value)
	require.NoError(t, err)
	return spell.NewFixedDiceSpellEffect(face, effectTurns(t, turns))
}

func effectTurns(t testing.TB, turns int) spell.EffectTurns {
	t.Helper()
	value, err := spell.NewEffectTurns(turns)
	require.NoError(t, err)
	return value
}

// 対象のクリーチャーにダメージの効果
func Damage(t testing.TB, amount int) spell.Effect {
	t.Helper()
	value, err := spell.NewDamageAmount(amount)
	require.NoError(t, err)
	return spell.NewDamageSpellEffect(value)
}
