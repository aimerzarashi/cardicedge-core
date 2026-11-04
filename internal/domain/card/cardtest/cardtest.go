package cardtest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 生成関数のエラーを毎回確かめると冗長なため、ここで確かめる

func Name(t testing.TB, value string) card.Name {
	t.Helper()
	name, err := card.NewName(value)
	require.NoError(t, err)
	return name
}

func OwnedQuantity(t testing.TB, value int) card.OwnedQuantity {
	t.Helper()
	quantity, err := card.NewOwnedQuantity(value)
	require.NoError(t, err)
	return quantity
}

func MaxDeckQuantity(t testing.TB, value int) card.MaxDeckQuantity {
	t.Helper()
	max, err := card.NewMaxDeckQuantity(value)
	require.NoError(t, err)
	return max
}

// 上限は、テストで超えることを確かめない限り、既定のルールと同じ 4 にそろえる
func DeckQuantity(t testing.TB, value int) card.DeckQuantity {
	t.Helper()
	return DeckQuantityWithin(t, value, MaxDeckQuantity(t, 4))
}

func DeckQuantityWithin(t testing.TB, value int, max card.MaxDeckQuantity) card.DeckQuantity {
	t.Helper()
	quantity, err := card.NewDeckQuantity(value, max)
	require.NoError(t, err)
	return quantity
}

func Ref(t testing.TB, entityType domain.EntityType, id int64) card.Ref {
	t.Helper()
	ref, err := card.NewRef(entityType, id)
	require.NoError(t, err)
	return ref
}

// Creature の内部 ID で指す
func CreatureRef(t testing.TB, id int64) card.Ref {
	t.Helper()
	return Ref(t, card.CreatureEntityType, id)
}

func DeckTotalCount(t testing.TB, value int) card.DeckTotalCount {
	t.Helper()
	count, err := card.NewDeckTotalCount(value)
	require.NoError(t, err)
	return count
}
