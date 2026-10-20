package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck/starterdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/inf/repository"
)

type starterDeckFixture struct {
	ctx      context.Context
	registry *domaindef.Registry
	writer   *repository.StarterDeckListWriter
}

// 件数・順番を確かめるため、ほかの行の無い DB（テンプレートの複製）で動かす
func newStarterDeckFixture(t *testing.T) starterDeckFixture {
	t.Helper()
	ctx := emptyCardLists(t)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return starterDeckFixture{
		ctx:      ctx,
		registry: registry,
		writer:   repository.NewStarterDeckListWriter(identifier.NewEntityRefResolver(registry)),
	}
}

// read model はカードを内部 ID で持つため、識別子を発行して、内部 ID と公開 ID の組を作る
func (f starterDeckFixture) issueCreature(t *testing.T) (starterdeck.CardRef, uuid.UUID) {
	t.Helper()
	id, publicID, err := identifier.New(f.registry, creature.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return starterdecktest.CardRef(t, creature.Types().EntityType, id.Int64()), publicID
}

func (f starterDeckFixture) issueItem(t *testing.T) (starterdeck.CardRef, uuid.UUID) {
	t.Helper()
	id, publicID, err := identifier.New(f.registry, item.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return starterdecktest.CardRef(t, item.Types().EntityType, id.Int64()), publicID
}

func starterDeckList(sortName, name string, cards map[starterdeck.CardRef]int) readmodel.StarterDeckList {
	id := uuid.New()
	at := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	metadata := readmodel.StarterDeckListMetadata{ID: id, Name: name, Revision: 3, RegisteredAt: at, UpdatedAt: at}
	return readmodel.StarterDeckList{StarterDeckID: id, SortName: sortName, Metadata: metadata, Cards: cards}
}

func (f starterDeckFixture) put(t *testing.T, sortName, name string) readmodel.StarterDeckList {
	t.Helper()
	list := starterDeckList(sortName, name, map[starterdeck.CardRef]int{})
	require.NoError(t, f.writer.Upsert(f.ctx, list))
	return list
}

func (f starterDeckFixture) listAll(t *testing.T) []query.StarterDeckListRow {
	t.Helper()
	rows, err := repository.ListStarterDecks(f.ctx, query.ListStarterDecksCondition{Limit: 100})
	require.NoError(t, err)
	return rows
}

func starterDeckNames(rows []query.StarterDeckListRow) []string {
	result := make([]string, len(rows))
	for i, row := range rows {
		result[i] = row.StarterDeck.Name
	}
	return result
}

// カードは内部 ID から公開 ID に引き直して書き、読むときは公開 ID と枚数で返す
func TestStarterDeckList_GetsUpsertedStarterDeckWithCardPublicIDs(t *testing.T) {
	f := newStarterDeckFixture(t)
	dragon, dragonID := f.issueCreature(t)
	potion, potionID := f.issueItem(t)
	list := starterDeckList("fire", "Ｆire", map[starterdeck.CardRef]int{dragon: 3, potion: 1})

	require.NoError(t, f.writer.Upsert(f.ctx, list))
	deck, err := repository.GetStarterDeck(f.ctx, list.StarterDeckID)

	require.NoError(t, err)
	require.Equal(t, list.StarterDeckID, deck.ID)
	require.Equal(t, "Ｆire", deck.Name)
	require.ElementsMatch(t, []query.StarterDeckCard{{CardID: dragonID, Count: 3}, {CardID: potionID, Count: 1}}, deck.Cards)
	require.Equal(t, 4, deck.TotalCount)
	require.Equal(t, int64(3), deck.Revision)
	require.Equal(t, list.Metadata.RegisteredAt, deck.RegisteredAt.UTC())
	require.Equal(t, list.Metadata.UpdatedAt, deck.UpdatedAt.UTC())
}

func TestStarterDeckList_GetFailsForMissingStarterDeck(t *testing.T) {
	f := newStarterDeckFixture(t)

	_, err := repository.GetStarterDeck(f.ctx, uuid.New())

	require.ErrorIs(t, err, query.ErrStarterDeckNotFound)
}

func TestStarterDeckList_UpsertOverwritesSameStarterDeck(t *testing.T) {
	f := newStarterDeckFixture(t)
	dragon, dragonID := f.issueCreature(t)
	list := f.put(t, "fire", "Fire")
	list.SortName = "water"
	list.Metadata.Name = "Water"
	list.Cards = map[starterdeck.CardRef]int{dragon: 2}

	require.NoError(t, f.writer.Upsert(f.ctx, list))

	rows := f.listAll(t)
	require.Equal(t, []string{"Water"}, starterDeckNames(rows))
	require.Equal(t, []query.StarterDeckCard{{CardID: dragonID, Count: 2}}, rows[0].StarterDeck.Cards)
}

func TestStarterDeckList_DeleteRemovesStarterDeck(t *testing.T) {
	f := newStarterDeckFixture(t)
	deleted := f.put(t, "fire", "Fire")
	f.put(t, "water", "Water")

	require.NoError(t, f.writer.Delete(f.ctx, deleted))

	require.Equal(t, []string{"Water"}, starterDeckNames(f.listAll(t)))
}

// 同じ集約を作り直すときに、何度呼ばれてもよいように
func TestStarterDeckList_DeleteAcceptsMissingStarterDeck(t *testing.T) {
	f := newStarterDeckFixture(t)

	err := f.writer.Delete(f.ctx, starterDeckList("fire", "Fire", map[starterdeck.CardRef]int{}))

	require.NoError(t, err)
}

func TestStarterDeckList_ListsBySortNameWithCursor(t *testing.T) {
	f := newStarterDeckFixture(t)
	f.put(t, "b", "B")
	first := f.put(t, "a", "A")
	f.put(t, "c", "C")
	after := query.StarterDeckListCursor{SortName: first.SortName, StarterDeckID: first.StarterDeckID}

	rows, err := repository.ListStarterDecks(f.ctx, query.ListStarterDecksCondition{After: &after, Limit: 1})

	require.NoError(t, err)
	require.Equal(t, []string{"A", "B", "C"}, starterDeckNames(f.listAll(t)))
	require.Equal(t, []string{"B"}, starterDeckNames(rows))
	require.Equal(t, query.StarterDeckListCursor{SortName: "b", StarterDeckID: rows[0].StarterDeck.ID}, rows[0].Cursor)
}

// LIKE の記号（% _）は文字どおりに比べる（カードの一覧と同じ）
func TestStarterDeckList_FiltersByNameContainsLiterally(t *testing.T) {
	f := newStarterDeckFixture(t)
	for _, name := range []string{"50%off", "50xoff"} {
		f.put(t, name, name)
	}

	rows, err := repository.ListStarterDecks(f.ctx, query.ListStarterDecksCondition{NameContains: "50%", Limit: 10})

	require.NoError(t, err)
	require.Equal(t, []string{"50%off"}, starterDeckNames(rows))
}
