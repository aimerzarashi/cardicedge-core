package npcdecktest

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 名前・カード・枚数はプリセットデッキの型なので、presetdecktest を使う

func ID(t testing.TB, value int64) npcdeck.ID {
	t.Helper()
	id, err := npcdeck.NewID(value)
	require.NoError(t, err)
	return id
}

func Source(t testing.TB, presetDeckID uuid.UUID, revision int64) npcdeck.Source {
	t.Helper()
	source, err := npcdeck.NewSource(presetDeckID, revision)
	require.NoError(t, err)
	return source
}
