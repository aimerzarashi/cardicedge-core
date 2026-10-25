package avatartest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
)

// テストでも値の型は生成関数で作る（factory_enforcer）

func ID(t testing.TB, value int64) avatar.ID {
	t.Helper()
	id, err := avatar.NewID(value)
	require.NoError(t, err)
	return id
}

func Name(t testing.TB, value string) avatar.Name {
	t.Helper()
	name, err := avatar.NewName(value)
	require.NoError(t, err)
	return name
}
