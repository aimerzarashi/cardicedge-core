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

type grantDeckFixture struct {
	presetDeckFixture
	starterWriter *repository.GrantDeckWriter
}

func newGrantDeckFixture(t *testing.T) grantDeckFixture {
	t.Helper()
	f := newPresetDeckFixture(t)
	return grantDeckFixture{presetDeckFixture: f, starterWriter: repository.NewGrantDeckWriter(identifier.NewEntityRefResolver(f.registry))}
}

func (f grantDeckFixture) grantDeckState(sortName, name string, status readmodel.GrantDeckStatus, cards map[presetdeck.CardRef]int) readmodel.GrantDeck {
	id := uuid.New()
	at := time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	metadata := readmodel.GrantDeckMetadata{ID: id, Name: name, Status: status, Revision: 3, RegisteredAt: at, UpdatedAt: at}
	return readmodel.GrantDeck{
		GrantDeckID: id, SortName: sortName, Status: status, Metadata: metadata, Cards: cards,
		RegisteredBy: f.registeredBy, UpdatedBy: f.updatedBy,
	}
}

func (f grantDeckFixture) put(t *testing.T, sortName, name string, status readmodel.GrantDeckStatus) readmodel.GrantDeck {
	t.Helper()
	deck := f.grantDeckState(sortName, name, status, map[presetdeck.CardRef]int{})
	require.NoError(t, f.starterWriter.Upsert(f.ctx, deck))
	return deck
}

func (f grantDeckFixture) list(t *testing.T, condition query.ListGrantDecksCondition) []query.GrantDeckListRow {
	t.Helper()
	rows, err := repository.NewGrantDeckReader().List(f.ctx, condition)
	require.NoError(t, err)
	return rows
}

func (f grantDeckFixture) listAll(t *testing.T) []query.GrantDeckListRow {
	t.Helper()
	return f.list(t, query.ListGrantDecksCondition{Limit: 100})
}

func grantDeckNames(rows []query.GrantDeckListRow) []string {
	result := make([]string, len(rows))
	for i, row := range rows {
		result[i] = row.GrantDeck.Name
	}
	return result
}

// カードは内部 ID から公開 ID に引き直して書き、読むときは公開 ID と枚数で返す。状態・写した元も読める
func TestGrantDeck_GetsUpsertedGrantDeckWithCardPublicIDsAndSource(t *testing.T) {
	f := newGrantDeckFixture(t)
	dragon, dragonID := f.issueCreature(t)
	potion, potionID := f.issueItem(t)
	deck := f.grantDeckState("fire", "Ｆire", readmodel.GrantDeckStatusPublished, map[presetdeck.CardRef]int{dragon: 3, potion: 1})
	deck.Metadata.Source = &readmodel.GrantDeckSource{PresetDeckID: uuid.New(), Revision: 7}

	require.NoError(t, f.starterWriter.Upsert(f.ctx, deck))
	got, err := repository.NewGrantDeckReader().Get(f.ctx, deck.GrantDeckID)

	require.NoError(t, err)
	require.Equal(t, deck.GrantDeckID, got.ID)
	require.Equal(t, "Ｆire", got.Name)
	require.Equal(t, readmodel.GrantDeckStatusPublished, got.Status)
	require.ElementsMatch(t, []query.PresetDeckCard{{CardID: dragonID, Count: 3}, {CardID: potionID, Count: 1}}, got.Cards)
	require.Equal(t, 4, got.TotalCount)
	require.Equal(t, &query.GrantDeckSource{PresetDeckID: deck.Metadata.Source.PresetDeckID, Revision: 7}, got.Source)
	require.Equal(t, int64(3), got.Revision)
	require.Equal(t, deck.Metadata.RegisteredAt, got.RegisteredAt.UTC())
	require.Equal(t, deck.Metadata.UpdatedAt, got.UpdatedAt.UTC())
	require.Equal(t, f.registeredByPublicID, got.RegisteredBy)
	require.Equal(t, f.updatedByPublicID, got.UpdatedBy)
}

func TestGrantDeck_GetsNoSourceForGrantDeckMadeFromScratch(t *testing.T) {
	f := newGrantDeckFixture(t)
	deck := f.put(t, "fire", "Fire", readmodel.GrantDeckStatusDraft)

	got, err := repository.NewGrantDeckReader().Get(f.ctx, deck.GrantDeckID)

	require.NoError(t, err)
	require.Nil(t, got.Source)
}

func TestGrantDeck_GetFailsForMissingGrantDeck(t *testing.T) {
	f := newGrantDeckFixture(t)

	_, err := repository.NewGrantDeckReader().Get(f.ctx, uuid.New())

	require.ErrorIs(t, err, query.ErrGrantDeckNotFound)
}

func TestGrantDeck_UpsertOverwritesSameGrantDeck(t *testing.T) {
	f := newGrantDeckFixture(t)
	dragon, dragonID := f.issueCreature(t)
	deck := f.put(t, "fire", "Fire", readmodel.GrantDeckStatusDraft)
	deck.SortName = "water"
	deck.Metadata.Name = "Water"
	deck.Status = readmodel.GrantDeckStatusRetired
	deck.Metadata.Status = readmodel.GrantDeckStatusRetired
	deck.Cards = map[presetdeck.CardRef]int{dragon: 2}

	require.NoError(t, f.starterWriter.Upsert(f.ctx, deck))

	rows := f.listAll(t)
	require.Equal(t, []string{"Water"}, grantDeckNames(rows))
	require.Equal(t, []query.PresetDeckCard{{CardID: dragonID, Count: 2}}, rows[0].GrantDeck.Cards)
	require.Equal(t, readmodel.GrantDeckStatusRetired, f.listAll(t)[0].GrantDeck.Status)
}

func TestGrantDeck_DeleteRemovesGrantDeckAndAcceptsMissingOne(t *testing.T) {
	f := newGrantDeckFixture(t)
	deleted := f.put(t, "fire", "Fire", readmodel.GrantDeckStatusDraft)
	f.put(t, "water", "Water", readmodel.GrantDeckStatusDraft)

	require.NoError(t, f.starterWriter.Delete(f.ctx, deleted))
	require.NoError(t, f.starterWriter.Delete(f.ctx, deleted))

	require.Equal(t, []string{"Water"}, grantDeckNames(f.listAll(t)))
}

func TestGrantDeck_ListsBySortNameWithCursor(t *testing.T) {
	f := newGrantDeckFixture(t)
	f.put(t, "b", "B", readmodel.GrantDeckStatusDraft)
	first := f.put(t, "a", "A", readmodel.GrantDeckStatusDraft)
	f.put(t, "c", "C", readmodel.GrantDeckStatusDraft)
	after := query.GrantDeckListCursor{SortName: first.SortName, GrantDeckID: first.GrantDeckID}

	rows := f.list(t, query.ListGrantDecksCondition{After: &after, Limit: 1})

	require.Equal(t, []string{"A", "B", "C"}, grantDeckNames(f.listAll(t)))
	require.Equal(t, []string{"B"}, grantDeckNames(rows))
	require.Equal(t, query.GrantDeckListCursor{SortName: "b", GrantDeckID: rows[0].GrantDeck.ID}, rows[0].Cursor)
}

func TestGrantDeck_FiltersByStatuses(t *testing.T) {
	f := newGrantDeckFixture(t)
	f.put(t, "a", "A", readmodel.GrantDeckStatusDraft)
	f.put(t, "b", "B", readmodel.GrantDeckStatusPublished)
	f.put(t, "c", "C", readmodel.GrantDeckStatusRetired)

	published := f.list(t, query.ListGrantDecksCondition{Statuses: []readmodel.GrantDeckStatus{readmodel.GrantDeckStatusPublished}, Limit: 10})
	notDraft := f.list(t, query.ListGrantDecksCondition{
		Statuses: []readmodel.GrantDeckStatus{readmodel.GrantDeckStatusPublished, readmodel.GrantDeckStatusRetired}, Limit: 10,
	})

	require.Equal(t, []string{"B"}, grantDeckNames(published))
	require.Equal(t, []string{"B", "C"}, grantDeckNames(notDraft))
}

// LIKE の記号（% _）は文字どおりに比べる（カードの一覧と同じ）
func TestGrantDeck_FiltersByNameContainsLiterally(t *testing.T) {
	f := newGrantDeckFixture(t)
	for _, name := range []string{"50%off", "50xoff"} {
		f.put(t, name, name, readmodel.GrantDeckStatusDraft)
	}

	rows := f.list(t, query.ListGrantDecksCondition{NameContains: "50%", Limit: 10})

	require.Equal(t, []string{"50%off"}, grantDeckNames(rows))
}
