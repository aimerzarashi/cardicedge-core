package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
)

type starterDeckFixture struct {
	presetDeckFixture
	starterWriter *repository.StarterDeckWriter
}

func newStarterDeckFixture(t *testing.T) starterDeckFixture {
	t.Helper()
	f := newPresetDeckFixture(t)
	return starterDeckFixture{presetDeckFixture: f, starterWriter: repository.NewStarterDeckWriter(identifier.NewEntityRefResolver(f.registry))}
}

func (f starterDeckFixture) starterDeckState(sortName, name string, status readmodel.StarterDeckStatus, cards map[presetdeck.CardRef]int) readmodel.StarterDeck {
	id := uuid.New()
	at := time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	metadata := readmodel.StarterDeckMetadata{ID: id, Name: name, Status: status, Revision: 3, RegisteredAt: at, UpdatedAt: at}
	return readmodel.StarterDeck{
		StarterDeckID: id, SortName: sortName, Status: status, Metadata: metadata, Cards: cards,
		RegisteredBy: f.registeredBy, UpdatedBy: f.updatedBy,
	}
}

func (f starterDeckFixture) put(t *testing.T, sortName, name string, status readmodel.StarterDeckStatus) readmodel.StarterDeck {
	t.Helper()
	deck := f.starterDeckState(sortName, name, status, map[presetdeck.CardRef]int{})
	require.NoError(t, f.starterWriter.Upsert(f.ctx, deck))
	return deck
}

func (f starterDeckFixture) list(t *testing.T, condition query.ListStarterDecksCondition) []query.StarterDeckListRow {
	t.Helper()
	rows, err := repository.NewStarterDeckReader().List(f.ctx, condition)
	require.NoError(t, err)
	return rows
}

func (f starterDeckFixture) listAll(t *testing.T) []query.StarterDeckListRow {
	t.Helper()
	return f.list(t, query.ListStarterDecksCondition{Limit: 100})
}

func starterDeckNames(rows []query.StarterDeckListRow) []string {
	result := make([]string, len(rows))
	for i, row := range rows {
		result[i] = row.StarterDeck.Name
	}
	return result
}

// カードは内部 ID から公開 ID に引き直して書き、読むときは公開 ID と枚数で返す。状態・写した元も読める
func TestStarterDeck_GetsUpsertedStarterDeckWithCardPublicIDsAndSource(t *testing.T) {
	f := newStarterDeckFixture(t)
	dragon, dragonID := f.issueCreature(t)
	potion, potionID := f.issueItem(t)
	deck := f.starterDeckState("fire", "Ｆire", readmodel.StarterDeckStatusPublished, map[presetdeck.CardRef]int{dragon: 3, potion: 1})
	deck.Metadata.Source = &readmodel.StarterDeckSource{PresetDeckID: uuid.New(), Revision: 7}

	require.NoError(t, f.starterWriter.Upsert(f.ctx, deck))
	got, err := repository.NewStarterDeckReader().Get(f.ctx, deck.StarterDeckID)

	require.NoError(t, err)
	require.Equal(t, deck.StarterDeckID, got.ID)
	require.Equal(t, "Ｆire", got.Name)
	require.Equal(t, readmodel.StarterDeckStatusPublished, got.Status)
	require.ElementsMatch(t, []query.PresetDeckCard{{CardID: dragonID, Count: 3}, {CardID: potionID, Count: 1}}, got.Cards)
	require.Equal(t, 4, got.TotalCount)
	require.Equal(t, &query.StarterDeckSource{PresetDeckID: deck.Metadata.Source.PresetDeckID, Revision: 7}, got.Source)
	require.Equal(t, int64(3), got.Revision)
	require.Equal(t, deck.Metadata.RegisteredAt, got.RegisteredAt.UTC())
	require.Equal(t, deck.Metadata.UpdatedAt, got.UpdatedAt.UTC())
	require.Equal(t, f.registeredByPublicID, got.RegisteredBy)
	require.Equal(t, f.updatedByPublicID, got.UpdatedBy)
}

func TestStarterDeck_GetsNoSourceForStarterDeckMadeFromScratch(t *testing.T) {
	f := newStarterDeckFixture(t)
	deck := f.put(t, "fire", "Fire", readmodel.StarterDeckStatusDraft)

	got, err := repository.NewStarterDeckReader().Get(f.ctx, deck.StarterDeckID)

	require.NoError(t, err)
	require.Nil(t, got.Source)
}

func TestStarterDeck_GetFailsForMissingStarterDeck(t *testing.T) {
	f := newStarterDeckFixture(t)

	_, err := repository.NewStarterDeckReader().Get(f.ctx, uuid.New())

	require.ErrorIs(t, err, query.ErrStarterDeckNotFound)
}

func TestStarterDeck_UpsertOverwritesSameStarterDeck(t *testing.T) {
	f := newStarterDeckFixture(t)
	dragon, dragonID := f.issueCreature(t)
	deck := f.put(t, "fire", "Fire", readmodel.StarterDeckStatusDraft)
	deck.SortName = "water"
	deck.Metadata.Name = "Water"
	deck.Status = readmodel.StarterDeckStatusRetired
	deck.Metadata.Status = readmodel.StarterDeckStatusRetired
	deck.Cards = map[presetdeck.CardRef]int{dragon: 2}

	require.NoError(t, f.starterWriter.Upsert(f.ctx, deck))

	rows := f.listAll(t)
	require.Equal(t, []string{"Water"}, starterDeckNames(rows))
	require.Equal(t, []query.PresetDeckCard{{CardID: dragonID, Count: 2}}, rows[0].StarterDeck.Cards)
	require.Equal(t, readmodel.StarterDeckStatusRetired, f.listAll(t)[0].StarterDeck.Status)
}

func TestStarterDeck_DeleteRemovesStarterDeckAndAcceptsMissingOne(t *testing.T) {
	f := newStarterDeckFixture(t)
	deleted := f.put(t, "fire", "Fire", readmodel.StarterDeckStatusDraft)
	f.put(t, "water", "Water", readmodel.StarterDeckStatusDraft)

	require.NoError(t, f.starterWriter.Delete(f.ctx, deleted))
	require.NoError(t, f.starterWriter.Delete(f.ctx, deleted))

	require.Equal(t, []string{"Water"}, starterDeckNames(f.listAll(t)))
}

func TestStarterDeck_ListsBySortNameWithCursor(t *testing.T) {
	f := newStarterDeckFixture(t)
	f.put(t, "b", "B", readmodel.StarterDeckStatusDraft)
	first := f.put(t, "a", "A", readmodel.StarterDeckStatusDraft)
	f.put(t, "c", "C", readmodel.StarterDeckStatusDraft)
	after := query.StarterDeckListCursor{SortName: first.SortName, StarterDeckID: first.StarterDeckID}

	rows := f.list(t, query.ListStarterDecksCondition{After: &after, Limit: 1})

	require.Equal(t, []string{"A", "B", "C"}, starterDeckNames(f.listAll(t)))
	require.Equal(t, []string{"B"}, starterDeckNames(rows))
	require.Equal(t, query.StarterDeckListCursor{SortName: "b", StarterDeckID: rows[0].StarterDeck.ID}, rows[0].Cursor)
}

func TestStarterDeck_FiltersByStatuses(t *testing.T) {
	f := newStarterDeckFixture(t)
	f.put(t, "a", "A", readmodel.StarterDeckStatusDraft)
	f.put(t, "b", "B", readmodel.StarterDeckStatusPublished)
	f.put(t, "c", "C", readmodel.StarterDeckStatusRetired)

	published := f.list(t, query.ListStarterDecksCondition{Statuses: []readmodel.StarterDeckStatus{readmodel.StarterDeckStatusPublished}, Limit: 10})
	notDraft := f.list(t, query.ListStarterDecksCondition{
		Statuses: []readmodel.StarterDeckStatus{readmodel.StarterDeckStatusPublished, readmodel.StarterDeckStatusRetired}, Limit: 10,
	})

	require.Equal(t, []string{"B"}, starterDeckNames(published))
	require.Equal(t, []string{"B", "C"}, starterDeckNames(notDraft))
}

// LIKE の記号（% _）は文字どおりに比べる（カードの一覧と同じ）
func TestStarterDeck_FiltersByNameContainsLiterally(t *testing.T) {
	f := newStarterDeckFixture(t)
	for _, name := range []string{"50%off", "50xoff"} {
		f.put(t, name, name, readmodel.StarterDeckStatusDraft)
	}

	rows := f.list(t, query.ListStarterDecksCondition{NameContains: "50%", Limit: 10})

	require.Equal(t, []string{"50%off"}, starterDeckNames(rows))
}
