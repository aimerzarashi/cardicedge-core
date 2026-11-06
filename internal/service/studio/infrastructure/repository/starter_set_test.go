package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck/grantdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar/npcavatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
)

type starterSetFixture struct {
	presetDeckFixture
	writer *repository.StarterSetWriter
	// read model はオペレーターを内部 ID で持つため、識別子を発行した 2 人（登録した人・最後に変えた人）
	registeredBy, updatedBy                 operator.ID
	registeredByPublicID, updatedByPublicID uuid.UUID
}

func newStarterSetFixture(t *testing.T) starterSetFixture {
	t.Helper()
	base := newPresetDeckFixture(t)
	f := starterSetFixture{presetDeckFixture: base, writer: repository.NewStarterSetWriter(identifier.NewEntityRefResolver(base.registry))}
	f.registeredBy, f.registeredByPublicID = f.issueOperator(t)
	f.updatedBy, f.updatedByPublicID = f.issueOperator(t)
	return f
}

func (f starterSetFixture) issueGrantDeck(t *testing.T) (grantdeck.ID, uuid.UUID) {
	t.Helper()
	id, publicID, err := identifier.New(f.registry, grantdeck.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id, publicID
}

func (f starterSetFixture) issueNPCAvatar(t *testing.T) (npcavatar.ID, uuid.UUID) {
	t.Helper()
	id, publicID, err := identifier.New(f.registry, npcavatar.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id, publicID
}

func (f starterSetFixture) issueBoard(t *testing.T) (board.ID, uuid.UUID) {
	t.Helper()
	id, publicID, err := identifier.New(f.registry, board.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id, publicID
}

func (f starterSetFixture) starterSetState(sortName, name string, status readmodel.StarterSetStatus) readmodel.StarterSet {
	id := uuid.New()
	at := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	metadata := readmodel.StarterSetMetadata{ID: id, Name: name, Status: status, Revision: 3, RegisteredAt: at, UpdatedAt: at}
	return readmodel.StarterSet{
		StarterSetID: id, SortName: sortName, Status: status, Metadata: metadata,
		RegisteredBy: f.registeredBy, UpdatedBy: f.updatedBy,
	}
}

func (f starterSetFixture) put(t *testing.T, sortName, name string, status readmodel.StarterSetStatus) readmodel.StarterSet {
	t.Helper()
	starterSet := f.starterSetState(sortName, name, status)
	require.NoError(t, f.writer.Upsert(f.ctx, starterSet))
	return starterSet
}

func (f starterSetFixture) list(t *testing.T, condition query.ListStarterSetsCondition) []query.StarterSetListRow {
	t.Helper()
	rows, err := repository.NewStarterSetReader().List(f.ctx, condition)
	require.NoError(t, err)
	return rows
}

func (f starterSetFixture) listAll(t *testing.T) []query.StarterSetListRow {
	t.Helper()
	return f.list(t, query.ListStarterSetsCondition{Limit: 100})
}

func starterSetNames(rows []query.StarterSetListRow) []string {
	result := make([]string, len(rows))
	for i, row := range rows {
		result[i] = row.StarterSet.Name
	}
	return result
}

func TestStarterSet_GetsUpsertedStarterSetWithOperatorPublicIDs(t *testing.T) {
	f := newStarterSetFixture(t)
	starterSet := f.starterSetState("fire", "Ｆire", readmodel.StarterSetStatusPublished)

	require.NoError(t, f.writer.Upsert(f.ctx, starterSet))
	got, err := repository.NewStarterSetReader().Get(f.ctx, starterSet.StarterSetID)

	require.NoError(t, err)
	require.Equal(t, starterSet.StarterSetID, got.ID)
	require.Equal(t, "Ｆire", got.Name)
	require.Equal(t, readmodel.StarterSetStatusPublished, got.Status)
	require.Equal(t, int64(3), got.Revision)
	require.Equal(t, starterSet.Metadata.RegisteredAt, got.RegisteredAt.UTC())
	require.Equal(t, starterSet.Metadata.UpdatedAt, got.UpdatedAt.UTC())
	require.Equal(t, f.registeredByPublicID, got.RegisteredBy)
	require.Equal(t, f.updatedByPublicID, got.UpdatedBy)
}

// 配るデッキと解放するボードは内部 ID で持ち、公開 ID に引き直して、持つ順のまま返す。何も指さないものは空
func TestStarterSet_GetsGrantDeckAndBoardPublicIDsInGivenOrder(t *testing.T) {
	f := newStarterSetFixture(t)
	firstDeck, firstDeckPublicID := f.issueGrantDeck(t)
	secondDeck, secondDeckPublicID := f.issueGrantDeck(t)
	firstBoard, firstBoardPublicID := f.issueBoard(t)
	secondBoard, secondBoardPublicID := f.issueBoard(t)
	firstNPC, firstNPCPublicID := f.issueNPCAvatar(t)
	secondNPC, secondNPCPublicID := f.issueNPCAvatar(t)
	starterSet := f.starterSetState("fire", "Ｆire", readmodel.StarterSetStatusDraft)
	starterSet.GrantDecks = []grantdeck.ID{secondDeck, firstDeck}
	starterSet.UnlockBoards = []board.ID{secondBoard, firstBoard}
	starterSet.UnlockNPCAvatars = []npcavatar.ID{secondNPC, firstNPC}
	empty := f.put(t, "water", "Water", readmodel.StarterSetStatusDraft)

	require.NoError(t, f.writer.Upsert(f.ctx, starterSet))
	got, err := repository.NewStarterSetReader().Get(f.ctx, starterSet.StarterSetID)
	gotEmpty, emptyErr := repository.NewStarterSetReader().Get(f.ctx, empty.StarterSetID)

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{secondDeckPublicID, firstDeckPublicID}, got.GrantDecks)
	require.Equal(t, []uuid.UUID{secondBoardPublicID, firstBoardPublicID}, got.UnlockBoards)
	require.Equal(t, []uuid.UUID{secondNPCPublicID, firstNPCPublicID}, got.UnlockNPCAvatars)
	require.NoError(t, emptyErr)
	require.Empty(t, gotEmpty.GrantDecks)
	require.Empty(t, gotEmpty.UnlockBoards)
	require.Empty(t, gotEmpty.UnlockNPCAvatars)
}

func TestStarterSet_UpsertFailsForGrantDeckOrBoardWithoutPublicID(t *testing.T) {
	f := newStarterSetFixture(t)
	withDeck := f.starterSetState("fire", "Ｆire", readmodel.StarterSetStatusDraft)
	withDeck.GrantDecks = []grantdeck.ID{grantdecktest.ID(t, 999999)}
	withBoard := f.starterSetState("water", "Water", readmodel.StarterSetStatusDraft)
	withBoard.UnlockBoards = []board.ID{boardtest.ID(t, 999999)}
	withNPC := f.starterSetState("earth", "Earth", readmodel.StarterSetStatusDraft)
	withNPC.UnlockNPCAvatars = []npcavatar.ID{npcavatartest.ID(t, 999999)}

	require.Error(t, f.writer.Upsert(f.ctx, withDeck))
	require.Error(t, f.writer.Upsert(f.ctx, withBoard))
	require.Error(t, f.writer.Upsert(f.ctx, withNPC))
}

func TestStarterSet_GetFailsForMissingStarterSet(t *testing.T) {
	f := newStarterSetFixture(t)

	_, err := repository.NewStarterSetReader().Get(f.ctx, uuid.New())

	require.ErrorIs(t, err, query.ErrStarterSetNotFound)
}

func TestStarterSet_UpsertOverwritesSameStarterSet(t *testing.T) {
	f := newStarterSetFixture(t)
	starterSet := f.put(t, "fire", "Fire", readmodel.StarterSetStatusDraft)
	starterSet.SortName = "water"
	starterSet.Metadata.Name = "Water"
	starterSet.Status = readmodel.StarterSetStatusRetired
	starterSet.Metadata.Status = readmodel.StarterSetStatusRetired

	require.NoError(t, f.writer.Upsert(f.ctx, starterSet))

	rows := f.listAll(t)
	require.Equal(t, []string{"Water"}, starterSetNames(rows))
	require.Equal(t, readmodel.StarterSetStatusRetired, rows[0].StarterSet.Status)
}

func TestStarterSet_DeleteRemovesStarterSetAndAcceptsMissingOne(t *testing.T) {
	f := newStarterSetFixture(t)
	deleted := f.put(t, "fire", "Fire", readmodel.StarterSetStatusDraft)
	f.put(t, "water", "Water", readmodel.StarterSetStatusDraft)

	require.NoError(t, f.writer.Delete(f.ctx, deleted))
	require.NoError(t, f.writer.Delete(f.ctx, deleted))

	require.Equal(t, []string{"Water"}, starterSetNames(f.listAll(t)))
}

func TestStarterSet_ListsBySortNameWithCursor(t *testing.T) {
	f := newStarterSetFixture(t)
	f.put(t, "b", "B", readmodel.StarterSetStatusDraft)
	first := f.put(t, "a", "A", readmodel.StarterSetStatusDraft)
	f.put(t, "c", "C", readmodel.StarterSetStatusDraft)
	after := query.StarterSetListCursor{SortName: first.SortName, StarterSetID: first.StarterSetID}

	rows := f.list(t, query.ListStarterSetsCondition{After: &after, Limit: 1})

	require.Equal(t, []string{"A", "B", "C"}, starterSetNames(f.listAll(t)))
	require.Equal(t, []string{"B"}, starterSetNames(rows))
	require.Equal(t, query.StarterSetListCursor{SortName: "b", StarterSetID: rows[0].StarterSet.ID}, rows[0].Cursor)
}

func TestStarterSet_FiltersByStatuses(t *testing.T) {
	f := newStarterSetFixture(t)
	f.put(t, "a", "A", readmodel.StarterSetStatusDraft)
	f.put(t, "b", "B", readmodel.StarterSetStatusPublished)
	f.put(t, "c", "C", readmodel.StarterSetStatusRetired)

	published := f.list(t, query.ListStarterSetsCondition{Statuses: []readmodel.StarterSetStatus{readmodel.StarterSetStatusPublished}, Limit: 10})
	notDraft := f.list(t, query.ListStarterSetsCondition{
		Statuses: []readmodel.StarterSetStatus{readmodel.StarterSetStatusPublished, readmodel.StarterSetStatusRetired}, Limit: 10,
	})

	require.Equal(t, []string{"B"}, starterSetNames(published))
	require.Equal(t, []string{"B", "C"}, starterSetNames(notDraft))
}

// LIKE の記号（% _）は文字どおりに比べる（カードの一覧と同じ）
func TestStarterSet_FiltersByNameContainsLiterally(t *testing.T) {
	f := newStarterSetFixture(t)
	for _, name := range []string{"50%off", "50xoff"} {
		f.put(t, name, name, readmodel.StarterSetStatusDraft)
	}

	rows := f.list(t, query.ListStarterSetsCondition{NameContains: "50%", Limit: 10})

	require.Equal(t, []string{"50%off"}, starterSetNames(rows))
}
