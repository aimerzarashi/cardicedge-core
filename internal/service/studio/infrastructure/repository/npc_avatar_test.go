package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
)

type npcAvatarFixture struct {
	presetDeckFixture
	writer *repository.NPCAvatarWriter
	// read model はオペレーターを内部 ID で持つため、識別子を発行した 2 人（登録した人・最後に変えた人）
	registeredBy, updatedBy                 operator.ID
	registeredByPublicID, updatedByPublicID uuid.UUID
}

func newNPCAvatarFixture(t *testing.T) npcAvatarFixture {
	t.Helper()
	base := newPresetDeckFixture(t)
	f := npcAvatarFixture{presetDeckFixture: base, writer: repository.NewNPCAvatarWriter(identifier.NewEntityRefResolver(base.registry))}
	f.registeredBy, f.registeredByPublicID = f.issueOperator(t)
	f.updatedBy, f.updatedByPublicID = f.issueOperator(t)
	return f
}

func (f npcAvatarFixture) npcAvatarState(sortName, name string, status readmodel.NPCAvatarStatus) readmodel.NPCAvatar {
	id := uuid.New()
	at := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	metadata := readmodel.NPCAvatarMetadata{ID: id, Name: name, Status: status, Revision: 3, RegisteredAt: at, UpdatedAt: at}
	return readmodel.NPCAvatar{
		NPCAvatarID: id, SortName: sortName, Status: status, Metadata: metadata,
		RegisteredBy: f.registeredBy, UpdatedBy: f.updatedBy,
	}
}

func (f npcAvatarFixture) put(t *testing.T, sortName, name string, status readmodel.NPCAvatarStatus) readmodel.NPCAvatar {
	t.Helper()
	npcAvatar := f.npcAvatarState(sortName, name, status)
	require.NoError(t, f.writer.Upsert(f.ctx, npcAvatar))
	return npcAvatar
}

func (f npcAvatarFixture) list(t *testing.T, condition query.ListNPCAvatarsCondition) []query.NPCAvatarListRow {
	t.Helper()
	rows, err := repository.NewNPCAvatarReader().List(f.ctx, condition)
	require.NoError(t, err)
	return rows
}

func (f npcAvatarFixture) listAll(t *testing.T) []query.NPCAvatarListRow {
	t.Helper()
	return f.list(t, query.ListNPCAvatarsCondition{Limit: 100})
}

func npcAvatarNames(rows []query.NPCAvatarListRow) []string {
	result := make([]string, len(rows))
	for i, row := range rows {
		result[i] = row.NPCAvatar.Name
	}
	return result
}

func TestNPCAvatar_GetsUpsertedNPCAvatarWithOperatorPublicIDs(t *testing.T) {
	f := newNPCAvatarFixture(t)
	npcAvatar := f.npcAvatarState("fire", "Ｆire", readmodel.NPCAvatarStatusPublished)

	require.NoError(t, f.writer.Upsert(f.ctx, npcAvatar))
	got, err := repository.NewNPCAvatarReader().Get(f.ctx, npcAvatar.NPCAvatarID)

	require.NoError(t, err)
	require.Equal(t, npcAvatar.NPCAvatarID, got.ID)
	require.Equal(t, "Ｆire", got.Name)
	require.Equal(t, readmodel.NPCAvatarStatusPublished, got.Status)
	require.Equal(t, int64(3), got.Revision)
	require.Equal(t, npcAvatar.Metadata.RegisteredAt, got.RegisteredAt.UTC())
	require.Equal(t, npcAvatar.Metadata.UpdatedAt, got.UpdatedAt.UTC())
	require.Equal(t, f.registeredByPublicID, got.RegisteredBy)
	require.Equal(t, f.updatedByPublicID, got.UpdatedBy)
}

func TestNPCAvatar_GetFailsForMissingNPCAvatar(t *testing.T) {
	f := newNPCAvatarFixture(t)

	_, err := repository.NewNPCAvatarReader().Get(f.ctx, uuid.New())

	require.ErrorIs(t, err, query.ErrNPCAvatarNotFound)
}

func TestNPCAvatar_UpsertOverwritesSameNPCAvatar(t *testing.T) {
	f := newNPCAvatarFixture(t)
	npcAvatar := f.put(t, "fire", "Fire", readmodel.NPCAvatarStatusDraft)
	npcAvatar.SortName = "water"
	npcAvatar.Metadata.Name = "Water"
	npcAvatar.Status = readmodel.NPCAvatarStatusRetired
	npcAvatar.Metadata.Status = readmodel.NPCAvatarStatusRetired

	require.NoError(t, f.writer.Upsert(f.ctx, npcAvatar))

	rows := f.listAll(t)
	require.Equal(t, []string{"Water"}, npcAvatarNames(rows))
	require.Equal(t, readmodel.NPCAvatarStatusRetired, rows[0].NPCAvatar.Status)
}

func TestNPCAvatar_DeleteRemovesNPCAvatarAndAcceptsMissingOne(t *testing.T) {
	f := newNPCAvatarFixture(t)
	deleted := f.put(t, "fire", "Fire", readmodel.NPCAvatarStatusDraft)
	f.put(t, "water", "Water", readmodel.NPCAvatarStatusDraft)

	require.NoError(t, f.writer.Delete(f.ctx, deleted))
	require.NoError(t, f.writer.Delete(f.ctx, deleted))

	require.Equal(t, []string{"Water"}, npcAvatarNames(f.listAll(t)))
}

func TestNPCAvatar_ListsBySortNameWithCursor(t *testing.T) {
	f := newNPCAvatarFixture(t)
	f.put(t, "b", "B", readmodel.NPCAvatarStatusDraft)
	first := f.put(t, "a", "A", readmodel.NPCAvatarStatusDraft)
	f.put(t, "c", "C", readmodel.NPCAvatarStatusDraft)
	after := query.NPCAvatarListCursor{SortName: first.SortName, NPCAvatarID: first.NPCAvatarID}

	rows := f.list(t, query.ListNPCAvatarsCondition{After: &after, Limit: 1})

	require.Equal(t, []string{"A", "B", "C"}, npcAvatarNames(f.listAll(t)))
	require.Equal(t, []string{"B"}, npcAvatarNames(rows))
	require.Equal(t, query.NPCAvatarListCursor{SortName: "b", NPCAvatarID: rows[0].NPCAvatar.ID}, rows[0].Cursor)
}

func TestNPCAvatar_FiltersByStatuses(t *testing.T) {
	f := newNPCAvatarFixture(t)
	f.put(t, "a", "A", readmodel.NPCAvatarStatusDraft)
	f.put(t, "b", "B", readmodel.NPCAvatarStatusPublished)
	f.put(t, "c", "C", readmodel.NPCAvatarStatusRetired)

	published := f.list(t, query.ListNPCAvatarsCondition{Statuses: []readmodel.NPCAvatarStatus{readmodel.NPCAvatarStatusPublished}, Limit: 10})
	notDraft := f.list(t, query.ListNPCAvatarsCondition{
		Statuses: []readmodel.NPCAvatarStatus{readmodel.NPCAvatarStatusPublished, readmodel.NPCAvatarStatusRetired}, Limit: 10,
	})

	require.Equal(t, []string{"B"}, npcAvatarNames(published))
	require.Equal(t, []string{"B", "C"}, npcAvatarNames(notDraft))
}

// LIKE の記号（% _）は文字どおりに比べる（カードの一覧と同じ）
func TestNPCAvatar_FiltersByNameContainsLiterally(t *testing.T) {
	f := newNPCAvatarFixture(t)
	for _, name := range []string{"50%off", "50xoff"} {
		f.put(t, name, name, readmodel.NPCAvatarStatusDraft)
	}

	rows := f.list(t, query.ListNPCAvatarsCondition{NameContains: "50%", Limit: 10})

	require.Equal(t, []string{"50%off"}, npcAvatarNames(rows))
}
