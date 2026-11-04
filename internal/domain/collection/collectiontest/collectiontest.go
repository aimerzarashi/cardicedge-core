package collectiontest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
)

// テストでも値の型は生成関数で作る（factory_enforcer）

func ID(t testing.TB, value int64) collection.ID {
	t.Helper()
	id, err := collection.NewID(value)
	require.NoError(t, err)
	return id
}

func Card(t testing.TB, ref card.Ref, quantity int) collection.Card {
	t.Helper()
	return collection.Card{Card: ref, Quantity: cardtest.OwnedQuantity(t, quantity)}
}
