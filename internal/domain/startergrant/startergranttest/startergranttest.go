package startergranttest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 生成関数のエラーを毎回確かめると冗長なため、ここで確かめる

func ID(t testing.TB, value int64) startergrant.ID {
	t.Helper()
	id, err := startergrant.NewID(value)
	require.NoError(t, err)
	return id
}

func DeckNumber(t testing.TB, value int) startergrant.DeckNumber {
	t.Helper()
	number, err := startergrant.NewDeckNumber(value)
	require.NoError(t, err)
	return number
}

func Deck(t testing.TB, name string, cards ...startergrant.DeckCard) startergrant.Deck {
	t.Helper()
	return startergrant.Deck{Name: DeckName(t, name), Cards: cards}
}

func DeckName(t testing.TB, value string) startergrant.DeckName {
	t.Helper()
	name, err := startergrant.NewDeckName(value)
	require.NoError(t, err)
	return name
}

func DeckCard(t testing.TB, ref card.Ref, quantity int) startergrant.DeckCard {
	t.Helper()
	return startergrant.DeckCard{Card: ref, Quantity: cardtest.DeckQuantity(t, quantity)}
}

func CollectionCard(t testing.TB, ref card.Ref, quantity int) startergrant.CollectionCard {
	t.Helper()
	return startergrant.CollectionCard{Card: ref, Quantity: cardtest.OwnedQuantity(t, quantity)}
}
