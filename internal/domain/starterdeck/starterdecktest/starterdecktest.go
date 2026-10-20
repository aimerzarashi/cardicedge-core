package starterdecktest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 生成関数のエラーを毎回確かめると冗長なため、ここで確かめる

func ID(t testing.TB, value int64) starterdeck.ID {
	t.Helper()
	id, err := starterdeck.NewID(value)
	require.NoError(t, err)
	return id
}

func Name(t testing.TB, value string) starterdeck.Name {
	t.Helper()
	name, err := starterdeck.NewName(value)
	require.NoError(t, err)
	return name
}

func CardCount(t testing.TB, value int) starterdeck.CardCount {
	t.Helper()
	count, err := starterdeck.NewCardCount(value)
	require.NoError(t, err)
	return count
}

func CardRef(t testing.TB, entityType domain.EntityType, id int64) starterdeck.CardRef {
	t.Helper()
	ref, err := starterdeck.NewCardRef(entityType, id)
	require.NoError(t, err)
	return ref
}
