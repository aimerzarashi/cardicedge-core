package npcdecktest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

// テストでも値の型は生成関数で作る（検証を通らない値でテストしないため。factory_enforcer）。
// 名前はプリセットデッキの型なので、presetdecktest を使う

func ID(t testing.TB, value int64) npcdeck.ID {
	t.Helper()
	id, err := npcdeck.NewID(value)
	require.NoError(t, err)
	return id
}

func Source(t testing.TB, presetDeckID presetdeck.ID, revision int64) npcdeck.Source {
	t.Helper()
	rev, err := presetdeck.NewRevision(revision)
	require.NoError(t, err)
	return npcdeck.Source{PresetDeck: presetDeckID, Revision: rev}
}
