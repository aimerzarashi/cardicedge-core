package creaturetest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 生成関数のエラーを毎回確かめると冗長なため、ここで確かめる

func ID(t testing.TB, value int64) creature.ID {
	t.Helper()
	id, err := creature.NewID(value)
	require.NoError(t, err)
	return id
}

func Elements(t testing.TB, values ...string) creature.Elements {
	t.Helper()
	elements, err := creature.NewElements(values)
	require.NoError(t, err)
	return elements
}

func Attack(t testing.TB, value int) creature.Attack {
	t.Helper()
	attack, err := creature.NewAttack(value)
	require.NoError(t, err)
	return attack
}

func ItemRestrictions(t testing.TB, values ...string) creature.ItemRestrictions {
	t.Helper()
	restrictions, err := creature.NewItemRestrictions(values)
	require.NoError(t, err)
	return restrictions
}

func PlacementRestrictions(t testing.TB, values ...string) creature.PlacementRestrictions {
	t.Helper()
	restrictions, err := creature.NewPlacementRestrictions(values)
	require.NoError(t, err)
	return restrictions
}

func HitPoints(t testing.TB, value int) creature.HitPoints {
	t.Helper()
	hitPoints, err := creature.NewHitPoints(value)
	require.NoError(t, err)
	return hitPoints
}
