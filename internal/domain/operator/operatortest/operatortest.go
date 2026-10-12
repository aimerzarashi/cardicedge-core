package operatortest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 生成関数のエラーを毎回確かめると冗長なため、ここで確かめる

func ID(t testing.TB, value int64) operator.ID {
	t.Helper()
	id, err := operator.NewID(value)
	require.NoError(t, err)
	return id
}

func Issuer(t testing.TB, value string) operator.Issuer {
	t.Helper()
	issuer, err := operator.NewIssuer(value)
	require.NoError(t, err)
	return issuer
}

func Subject(t testing.TB, value string) operator.Subject {
	t.Helper()
	subject, err := operator.NewSubject(value)
	require.NoError(t, err)
	return subject
}

func Email(t testing.TB, value string) operator.Email {
	t.Helper()
	email, err := operator.NewEmail(value)
	require.NoError(t, err)
	return email
}
