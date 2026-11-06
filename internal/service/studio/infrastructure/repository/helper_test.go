package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
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
	// read model はオペレーターを内部 ID で持つため、識別子を発行した 2 人（登録した人・最後に変えた人）。書いたあとは公開 ID で読める
	registeredBy, updatedBy                 operator.ID
	registeredByPublicID, updatedByPublicID uuid.UUID
}

// 件数・順番を確かめるため、ほかの行の無い DB（テンプレートの複製）で動かす
func newPresetDeckFixture(t *testing.T) presetDeckFixture {
	t.Helper()
	ctx := emptyCardLists(t)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	f := presetDeckFixture{
		ctx:      ctx,
		registry: registry,
		writer:   repository.NewPresetDeckWriter(identifier.NewEntityRefResolver(registry)),
	}
	f.registeredBy, f.registeredByPublicID = f.issueOperator(t)
	f.updatedBy, f.updatedByPublicID = f.issueOperator(t)
	return f
}

// read model はオペレーターを内部 ID で持つため、識別子を発行して、内部 ID と公開 ID の組を作る
func (f presetDeckFixture) issueOperator(t *testing.T) (operator.ID, uuid.UUID) {
	t.Helper()
	id, publicID, err := identifier.New(f.registry, operator.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id, publicID
}

// read model はカードを内部 ID で持つため、識別子を発行して、内部 ID と公開 ID の組を作る
func (f presetDeckFixture) issueCreature(t *testing.T) (card.Ref, uuid.UUID) {
	t.Helper()
	id, publicID, err := identifier.New(f.registry, creature.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return cardtest.Ref(t, creature.Types().EntityType, id.Int64()), publicID
}

func (f presetDeckFixture) issueItem(t *testing.T) (card.Ref, uuid.UUID) {
	t.Helper()
	id, publicID, err := identifier.New(f.registry, item.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return cardtest.Ref(t, item.Types().EntityType, id.Int64()), publicID
}

func (f presetDeckFixture) presetDeckList(sortName, name string, cards map[card.Ref]int) readmodel.PresetDeck {
	id := uuid.New()
	at := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	metadata := readmodel.PresetDeckMetadata{ID: id, Name: name, Revision: 3, RegisteredAt: at, UpdatedAt: at}
	return readmodel.PresetDeck{
		PresetDeckID: id, SortName: sortName, Metadata: metadata, Cards: cards,
		RegisteredBy: f.registeredBy, UpdatedBy: f.updatedBy,
	}
}

func (f presetDeckFixture) put(t *testing.T, sortName, name string) readmodel.PresetDeck {
	t.Helper()
	list := f.presetDeckList(sortName, name, map[card.Ref]int{})
	require.NoError(t, f.writer.Upsert(f.ctx, list))
	return list
}
