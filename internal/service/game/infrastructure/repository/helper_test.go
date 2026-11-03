package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

type playerFixture struct {
	ctx      context.Context
	registry *domaindef.Registry
}

// 件数を確かめるため、ほかの行の無い DB（テンプレートの複製）で動かす
func newPlayerFixture(t *testing.T) playerFixture {
	t.Helper()
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return playerFixture{ctx: ctx, registry: registry}
}

// 配布の投影はプレイヤーを内部 ID で受け取り公開用の ID を引くため、識別子を発行して組を作る
func (f playerFixture) issuePlayer(t *testing.T) (player.ID, uuid.UUID) {
	t.Helper()
	id, publicID, err := identifier.New(f.registry, player.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id, publicID
}

// デッキの投影はデッキを内部 ID で受け取り公開用の ID を引くため、識別子を発行して組を作る
func (f playerFixture) issueDeck(t *testing.T) (deck.ID, uuid.UUID) {
	t.Helper()
	id, publicID, err := identifier.New(f.registry, deck.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id, publicID
}
