package repository_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/unlock"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
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

// 解放の投影は解放を内部 ID で受け取り公開用の ID を引くため、識別子を発行して組を作る
func (f playerFixture) issueUnlock(t *testing.T) (unlock.ID, uuid.UUID) {
	t.Helper()
	id, publicID, err := identifier.New(f.registry, unlock.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id, publicID
}

// 部屋が名前を引く元の行を、公開 ID を指定して入れる（公開中。名前以外の項目は部屋の表示に使わない）

func insertNamedBoard(t *testing.T, ctx context.Context, id uuid.UUID, name string) {
	t.Helper()
	metadata := boardMetadata(name, readmodel.BoardStatusPublished)
	metadata.ID = id
	insertBoard(t, ctx, name, metadata)
}

func insertNamedNPCAvatar(t *testing.T, ctx context.Context, id uuid.UUID, name string) {
	t.Helper()
	metadata := npcAvatarMetadata(name, readmodel.NPCAvatarStatusPublished)
	metadata.ID = id
	insertNPCAvatar(t, ctx, name, metadata)
}

func insertNamedNPCMindset(t *testing.T, ctx context.Context, id uuid.UUID, name string) {
	t.Helper()
	raw, err := json.Marshal(readmodel.NPCMindsetMetadata{ID: id, Name: name, Status: readmodel.NPCMindsetStatusPublished})
	require.NoError(t, err)
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	row := models.NPCMindset{NPCMindsetID: id.String(), SortName: name, Status: string(readmodel.NPCMindsetStatusPublished), Metadata: raw}
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
}

func insertNamedNPCDeck(t *testing.T, ctx context.Context, id uuid.UUID, name string) {
	t.Helper()
	raw, err := json.Marshal(readmodel.NPCDeckMetadata{ID: id, Name: name, Status: readmodel.NPCDeckStatusPublished, Cards: []readmodel.DeckCard{}})
	require.NoError(t, err)
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	row := models.NPCDeck{NPCDeckID: id.String(), SortName: name, Status: string(readmodel.NPCDeckStatusPublished), Metadata: raw}
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
}
