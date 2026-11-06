package unlocktest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/unlock"
)

// テストでも値の型は生成関数で作る（factory_enforcer）

func ID(t testing.TB, value int64) unlock.ID {
	t.Helper()
	id, err := unlock.NewID(value)
	require.NoError(t, err)
	return id
}
