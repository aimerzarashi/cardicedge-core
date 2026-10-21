package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck/presetdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/inf/repository"
)

// 投影が書いたものを、問い合わせが読む形で確かめる（表の作りをテストに書かない）。
// 件数・順番を確かめるため、ほかの行の無い DB（テンプレートの複製）で動かす
func emptyCardLists(t *testing.T) context.Context {
	t.Helper()
	db, _ := rdbtest.NewDatabase(t)
	return rdb.WithDB(t.Context(), db)
}

type presetDeckFixture struct {
	ctx      context.Context
	registry *domaindef.Registry
	writer   *repository.PresetDeckWriter
}

// 件数・順番を確かめるため、ほかの行の無い DB（テンプレートの複製）で動かす
func newPresetDeckFixture(t *testing.T) presetDeckFixture {
	t.Helper()
	ctx := emptyCardLists(t)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return presetDeckFixture{
		ctx:      ctx,
		registry: registry,
		writer:   repository.NewPresetDeckWriter(identifier.NewEntityRefResolver(registry)),
	}
}

// read model はカードを内部 ID で持つため、識別子を発行して、内部 ID と公開 ID の組を作る
func (f presetDeckFixture) issueCreature(t *testing.T) (presetdeck.CardRef, uuid.UUID) {
	t.Helper()
	id, publicID, err := identifier.New(f.registry, creature.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return presetdecktest.CardRef(t, creature.Types().EntityType, id.Int64()), publicID
}

func (f presetDeckFixture) issueItem(t *testing.T) (presetdeck.CardRef, uuid.UUID) {
	t.Helper()
	id, publicID, err := identifier.New(f.registry, item.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return presetdecktest.CardRef(t, item.Types().EntityType, id.Int64()), publicID
}

func presetDeckList(sortName, name string, cards map[presetdeck.CardRef]int) readmodel.PresetDeck {
	id := uuid.New()
	at := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	metadata := readmodel.PresetDeckMetadata{ID: id, Name: name, Revision: 3, RegisteredAt: at, UpdatedAt: at}
	return readmodel.PresetDeck{PresetDeckID: id, SortName: sortName, Metadata: metadata, Cards: cards}
}

func (f presetDeckFixture) put(t *testing.T, sortName, name string) readmodel.PresetDeck {
	t.Helper()
	list := presetDeckList(sortName, name, map[presetdeck.CardRef]int{})
	require.NoError(t, f.writer.Upsert(f.ctx, list))
	return list
}
