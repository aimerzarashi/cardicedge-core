package grantdecktest

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 名前・カード・枚数はプリセットデッキの型なので、presetdecktest を使う

func ID(t testing.TB, value int64) grantdeck.ID {
	t.Helper()
	id, err := grantdeck.NewID(value)
	require.NoError(t, err)
	return id
}

func Source(t testing.TB, presetDeckID uuid.UUID, revision int64) grantdeck.Source {
	t.Helper()
	source, err := grantdeck.NewSource(presetDeckID, revision)
	require.NoError(t, err)
	return source
}
