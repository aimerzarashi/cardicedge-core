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
