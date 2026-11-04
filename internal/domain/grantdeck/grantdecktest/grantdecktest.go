package grantdecktest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 名前はプリセットデッキの型なので、presetdecktest を使う

func ID(t testing.TB, value int64) grantdeck.ID {
	t.Helper()
	id, err := grantdeck.NewID(value)
	require.NoError(t, err)
	return id
}

func Source(t testing.TB, presetDeckID presetdeck.ID, revision int64) grantdeck.Source {
	t.Helper()
	rev, err := presetdeck.NewRevision(revision)
	require.NoError(t, err)
	return grantdeck.Source{PresetDeck: presetDeckID, Revision: rev}
}
