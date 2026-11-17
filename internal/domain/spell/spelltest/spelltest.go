package spelltest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect"
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
func DrawCards(t testing.TB, count int) effect.SpellEffect {
	t.Helper()
	drawCount, err := effect.NewDrawCount(count)
	require.NoError(t, err)
	return effect.NewDrawCardsSpellEffect(drawCount)
}

// 魔力を得る効果
func GainMagic(t testing.TB, perLap int) effect.SpellEffect {
	t.Helper()
	value, err := effect.NewMagicPerLap(perLap)
	require.NoError(t, err)
	return effect.NewGainMagicSpellEffect(value)
}

// サイコロを複数振る効果
func MultipleDice(t testing.TB, count, turns int) effect.SpellEffect {
	t.Helper()
	value, err := effect.NewDiceCount(count)
	require.NoError(t, err)
	return effect.NewMultipleDiceSpellEffect(value, effectTurns(t, turns))
}

// サイコロの目を固定する効果
func FixedDice(t testing.TB, value, turns int) effect.SpellEffect {
	t.Helper()
	face, err := effect.NewDiceFace(value)
	require.NoError(t, err)
	return effect.NewFixedDiceSpellEffect(face, effectTurns(t, turns))
}

func effectTurns(t testing.TB, turns int) effect.EffectTurns {
	t.Helper()
	value, err := effect.NewEffectTurns(turns)
	require.NoError(t, err)
	return value
}

// 対象のクリーチャーにダメージの効果
func Damage(t testing.TB, amount int) effect.SpellEffect {
	t.Helper()
	value, err := effect.NewDamageAmount(amount)
	require.NoError(t, err)
	return effect.NewDamageSpellEffect(value)
}
