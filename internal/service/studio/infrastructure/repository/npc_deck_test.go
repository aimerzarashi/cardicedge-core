package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
)

type npcDeckFixture struct {
	presetDeckFixture
	npcWriter *repository.NPCDeckWriter
	// read model はオペレーターを内部 ID で持つため、識別子を発行した 2 人（登録した人・最後に変えた人）
	registeredBy, updatedBy                 operator.ID
	registeredByPublicID, updatedByPublicID uuid.UUID
}

func newNPCDeckFixture(t *testing.T) npcDeckFixture {
	t.Helper()
	base := newPresetDeckFixture(t)
	f := npcDeckFixture{presetDeckFixture: base, npcWriter: repository.NewNPCDeckWriter(identifier.NewEntityRefResolver(base.registry))}
	f.registeredBy, f.registeredByPublicID = f.issueOperator(t)
	f.updatedBy, f.updatedByPublicID = f.issueOperator(t)
	return f
}

func (f npcDeckFixture) npcDeckState(sortName, name string, status readmodel.NPCDeckStatus, cards map[card.Ref]int) readmodel.NPCDeck {
	id := uuid.New()
	at := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	metadata := readmodel.NPCDeckMetadata{ID: id, Name: name, Status: status, Revision: 3, RegisteredAt: at, UpdatedAt: at}
	return readmodel.NPCDeck{
		NPCDeckID: id, SortName: sortName, Status: status, Metadata: metadata, Cards: cards,
		RegisteredBy: f.registeredBy, UpdatedBy: f.updatedBy,
	}
}

func (f npcDeckFixture) put(t *testing.T, sortName, name string, status readmodel.NPCDeckStatus) readmodel.NPCDeck {
	t.Helper()
	npcDeck := f.npcDeckState(sortName, name, status, map[card.Ref]int{})
	require.NoError(t, f.npcWriter.Upsert(f.ctx, npcDeck))
	return npcDeck
}

func (f npcDeckFixture) list(t *testing.T, condition query.ListNPCDecksCondition) []query.NPCDeckListRow {
	t.Helper()
	rows, err := repository.NewNPCDeckReader().List(f.ctx, condition)
	require.NoError(t, err)
	return rows
}

func (f npcDeckFixture) listAll(t *testing.T) []query.NPCDeckListRow {
	t.Helper()
	return f.list(t, query.ListNPCDecksCondition{Limit: 100})
}

func npcDeckNames(rows []query.NPCDeckListRow) []string {
	result := make([]string, len(rows))
	for i, row := range rows {
		result[i] = row.NPCDeck.Name
	}
	return result
}

// カードは内部 ID から公開 ID に引き直して書き、読むときは公開 ID と枚数で返す。状態も読める
func TestNPCDeck_GetsUpsertedNPCDeckWithCardPublicIDs(t *testing.T) {
	f := newNPCDeckFixture(t)
	dragon, dragonID := f.issueCreature(t)
	potion, potionID := f.issueItem(t)
	npcDeck := f.npcDeckState("fire", "Ｆire", readmodel.NPCDeckStatusPublished, map[card.Ref]int{dragon: 3, potion: 1})

	require.NoError(t, f.npcWriter.Upsert(f.ctx, npcDeck))
	got, err := repository.NewNPCDeckReader().Get(f.ctx, npcDeck.NPCDeckID)

	require.NoError(t, err)
	require.Equal(t, npcDeck.NPCDeckID, got.ID)
	require.Equal(t, "Ｆire", got.Name)
	require.Equal(t, readmodel.NPCDeckStatusPublished, got.Status)
	require.ElementsMatch(t, []query.PresetDeckCard{{CardID: dragonID, Count: 3}, {CardID: potionID, Count: 1}}, got.Cards)
	require.Equal(t, 4, got.TotalCount)
	require.Equal(t, int64(3), got.Revision)
	require.Equal(t, npcDeck.Metadata.RegisteredAt, got.RegisteredAt.UTC())
	require.Equal(t, npcDeck.Metadata.UpdatedAt, got.UpdatedAt.UTC())
	require.Equal(t, f.registeredByPublicID, got.RegisteredBy)
	require.Equal(t, f.updatedByPublicID, got.UpdatedBy)
}

func TestNPCDeck_GetFailsForMissingNPCDeck(t *testing.T) {
	f := newNPCDeckFixture(t)

	_, err := repository.NewNPCDeckReader().Get(f.ctx, uuid.New())

	require.ErrorIs(t, err, query.ErrNPCDeckNotFound)
}

func TestNPCDeck_UpsertOverwritesSameNPCDeck(t *testing.T) {
	f := newNPCDeckFixture(t)
	dragon, dragonID := f.issueCreature(t)
	npcDeck := f.put(t, "fire", "Fire", readmodel.NPCDeckStatusDraft)
	npcDeck.SortName = "water"
	npcDeck.Metadata.Name = "Water"
	npcDeck.Status = readmodel.NPCDeckStatusRetired
	npcDeck.Metadata.Status = readmodel.NPCDeckStatusRetired
	npcDeck.Cards = map[card.Ref]int{dragon: 2}

	require.NoError(t, f.npcWriter.Upsert(f.ctx, npcDeck))

	rows := f.listAll(t)
	require.Equal(t, []string{"Water"}, npcDeckNames(rows))
	require.Equal(t, []query.PresetDeckCard{{CardID: dragonID, Count: 2}}, rows[0].NPCDeck.Cards)
	require.Equal(t, readmodel.NPCDeckStatusRetired, f.listAll(t)[0].NPCDeck.Status)
}

func TestNPCDeck_DeleteRemovesNPCDeckAndAcceptsMissingOne(t *testing.T) {
	f := newNPCDeckFixture(t)
	deleted := f.put(t, "fire", "Fire", readmodel.NPCDeckStatusDraft)
	f.put(t, "water", "Water", readmodel.NPCDeckStatusDraft)

	require.NoError(t, f.npcWriter.Delete(f.ctx, deleted))
	require.NoError(t, f.npcWriter.Delete(f.ctx, deleted))

	require.Equal(t, []string{"Water"}, npcDeckNames(f.listAll(t)))
}

func TestNPCDeck_ListsBySortNameWithCursor(t *testing.T) {
	f := newNPCDeckFixture(t)
	f.put(t, "b", "B", readmodel.NPCDeckStatusDraft)
	first := f.put(t, "a", "A", readmodel.NPCDeckStatusDraft)
	f.put(t, "c", "C", readmodel.NPCDeckStatusDraft)
	after := query.NPCDeckListCursor{SortName: first.SortName, NPCDeckID: first.NPCDeckID}

	rows := f.list(t, query.ListNPCDecksCondition{After: &after, Limit: 1})

	require.Equal(t, []string{"A", "B", "C"}, npcDeckNames(f.listAll(t)))
	require.Equal(t, []string{"B"}, npcDeckNames(rows))
	require.Equal(t, query.NPCDeckListCursor{SortName: "b", NPCDeckID: rows[0].NPCDeck.ID}, rows[0].Cursor)
}

func TestNPCDeck_FiltersByStatuses(t *testing.T) {
	f := newNPCDeckFixture(t)
	f.put(t, "a", "A", readmodel.NPCDeckStatusDraft)
	f.put(t, "b", "B", readmodel.NPCDeckStatusPublished)
	f.put(t, "c", "C", readmodel.NPCDeckStatusRetired)

	published := f.list(t, query.ListNPCDecksCondition{Statuses: []readmodel.NPCDeckStatus{readmodel.NPCDeckStatusPublished}, Limit: 10})
	notDraft := f.list(t, query.ListNPCDecksCondition{
		Statuses: []readmodel.NPCDeckStatus{readmodel.NPCDeckStatusPublished, readmodel.NPCDeckStatusRetired}, Limit: 10,
	})

	require.Equal(t, []string{"B"}, npcDeckNames(published))
	require.Equal(t, []string{"B", "C"}, npcDeckNames(notDraft))
}

// LIKE の記号（% _）は文字どおりに比べる（カードの一覧と同じ）
func TestNPCDeck_FiltersByNameContainsLiterally(t *testing.T) {
	f := newNPCDeckFixture(t)
	for _, name := range []string{"50%off", "50xoff"} {
		f.put(t, name, name, readmodel.NPCDeckStatusDraft)
	}

	rows := f.list(t, query.ListNPCDecksCondition{NameContains: "50%", Limit: 10})

	require.Equal(t, []string{"50%off"}, npcDeckNames(rows))
}
