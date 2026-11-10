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
	return spell.NewDrawCardsEffect(drawCount)
}

// 魔力を得る効果
func GainMagic(t testing.TB, perLap int) spell.Effect {
	t.Helper()
	value, err := spell.NewMagicPerLap(perLap)
	require.NoError(t, err)
	return spell.NewGainMagicEffect(value)
}

// サイコロを複数振る効果
func RollDice(t testing.TB, count int) spell.Effect {
	t.Helper()
	value, err := spell.NewDiceCount(count)
	require.NoError(t, err)
	return spell.NewRollDiceEffect(value)
}
