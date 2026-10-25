package starterdecksettest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 生成関数のエラーを毎回確かめると冗長なため、ここで確かめる

func ID(t testing.TB, value int64) starterdeckset.ID {
	t.Helper()
	id, err := starterdeckset.NewID(value)
	require.NoError(t, err)
	return id
}

func Deck(t testing.TB, deck presetdeck.PresetDeck, rules gamerules.Deck) starterdeckset.Deck {
	t.Helper()
	copied, err := starterdeckset.NewDeck(deck, rules)
	require.NoError(t, err)
	return copied
}
