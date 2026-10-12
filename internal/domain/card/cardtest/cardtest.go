package cardtest

import (
	"testing"

	"github.com/stretchr/testify/require"

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
