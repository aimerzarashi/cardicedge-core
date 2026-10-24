package playertest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 生成関数のエラーを毎回確かめると冗長なため、ここで確かめる

func ID(t testing.TB, value int64) player.ID {
	t.Helper()
	id, err := player.NewID(value)
	require.NoError(t, err)
	return id
}

func Issuer(t testing.TB, value string) player.Issuer {
	t.Helper()
	issuer, err := player.NewIssuer(value)
	require.NoError(t, err)
	return issuer
}

func Subject(t testing.TB, value string) player.Subject {
	t.Helper()
	subject, err := player.NewSubject(value)
	require.NoError(t, err)
	return subject
}
