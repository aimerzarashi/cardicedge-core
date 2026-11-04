package decktest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
)

// テストでも値の型は生成関数で作る（factory_enforcer）

func ID(t testing.TB, value int64) deck.ID {
	t.Helper()
	id, err := deck.NewID(value)
	require.NoError(t, err)
	return id
}

func Name(t testing.TB, value string) deck.Name {
	t.Helper()
	name, err := deck.NewName(value)
	require.NoError(t, err)
	return name
}

func Card(t testing.TB, ref card.Ref, quantity int) deck.Card {
	t.Helper()
	return deck.Card{Card: ref, Quantity: cardtest.DeckQuantity(t, quantity)}
}
