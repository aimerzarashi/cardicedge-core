package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/matchaction"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
)

type npcMindsetFixture struct {
	presetDeckFixture
	writer *repository.NPCMindsetWriter
	// read model はオペレーターを内部 ID で持つため、識別子を発行した 2 人（登録した人・最後に変えた人）
	registeredBy, updatedBy                 operator.ID
	registeredByPublicID, updatedByPublicID uuid.UUID
}

func newNPCMindsetFixture(t *testing.T) npcMindsetFixture {
	t.Helper()
	base := newPresetDeckFixture(t)
	f := npcMindsetFixture{presetDeckFixture: base, writer: repository.NewNPCMindsetWriter(identifier.NewEntityRefResolver(base.registry))}
	f.registeredBy, f.registeredByPublicID = f.issueOperator(t)
	f.updatedBy, f.updatedByPublicID = f.issueOperator(t)
	return f
}

func (f npcMindsetFixture) npcMindsetState(sortName, name string, status readmodel.NPCMindsetStatus) readmodel.NPCMindset {
	id := uuid.New()
	at := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	metadata := readmodel.NPCMindsetMetadata{
		ID: id, Name: name, Status: status, Revision: 3, RegisteredAt: at, UpdatedAt: at,
		ActionWeights: map[matchaction.Action]int{}, CriterionWeights: map[npcmindset.Criterion]int{},
	}
	for _, action := range matchaction.Actions() {
		metadata.ActionWeights[action] = 1
	}
	for _, criterion := range npcmindset.Criteria() {
		metadata.CriterionWeights[criterion] = 1
	}
	return readmodel.NPCMindset{
		NPCMindsetID: id, SortName: sortName, Status: status, Metadata: metadata,
		RegisteredBy: f.registeredBy, UpdatedBy: f.updatedBy,
	}
}

func (f npcMindsetFixture) put(t *testing.T, sortName, name string, status readmodel.NPCMindsetStatus) readmodel.NPCMindset {
	t.Helper()
	npcMindset := f.npcMindsetState(sortName, name, status)
	require.NoError(t, f.writer.Upsert(f.ctx, npcMindset))
	return npcMindset
}

func (f npcMindsetFixture) list(t *testing.T, condition query.ListNPCMindsetsCondition) []query.NPCMindsetListRow {
	t.Helper()
	rows, err := repository.NewNPCMindsetReader().List(f.ctx, condition)
	require.NoError(t, err)
	return rows
}

func (f npcMindsetFixture) listAll(t *testing.T) []query.NPCMindsetListRow {
	t.Helper()
	return f.list(t, query.ListNPCMindsetsCondition{Limit: 100})
}

func npcMindsetNames(rows []query.NPCMindsetListRow) []string {
	result := make([]string, len(rows))
	for i, row := range rows {
		result[i] = row.NPCMindset.Name
	}
	return result
}

func TestNPCMindset_GetsUpsertedNPCMindsetWithOperatorPublicIDs(t *testing.T) {
	f := newNPCMindsetFixture(t)
	npcMindset := f.npcMindsetState("fire", "Ｆire", readmodel.NPCMindsetStatusPublished)

	require.NoError(t, f.writer.Upsert(f.ctx, npcMindset))
	got, err := repository.NewNPCMindsetReader().Get(f.ctx, npcMindset.NPCMindsetID)

	require.NoError(t, err)
	require.Equal(t, npcMindset.NPCMindsetID, got.ID)
	require.Equal(t, "Ｆire", got.Name)
	require.Equal(t, readmodel.NPCMindsetStatusPublished, got.Status)
	require.Equal(t, int64(3), got.Revision)
	require.Equal(t, npcMindset.Metadata.RegisteredAt, got.RegisteredAt.UTC())
	require.Equal(t, npcMindset.Metadata.UpdatedAt, got.UpdatedAt.UTC())
	require.Equal(t, f.registeredByPublicID, got.RegisteredBy)
	require.Equal(t, f.updatedByPublicID, got.UpdatedBy)
}

// 重みは行動の種類・採点基準の定義の順に並べて返す（map の順は不定なので）。種類・基準のすべてを持たない行は読み出しの誤り
func TestNPCMindset_GetsWeightsInDefinitionOrder(t *testing.T) {
	f := newNPCMindsetFixture(t)
	npcMindset := f.npcMindsetState("fire", "Ｆire", readmodel.NPCMindsetStatusDraft)
	npcMindset.Metadata.ActionWeights[matchaction.ActionEndTurn] = 7
	npcMindset.Metadata.CriterionWeights[npcmindset.CriterionMagicReserve] = 3

	require.NoError(t, f.writer.Upsert(f.ctx, npcMindset))
	got, err := repository.NewNPCMindsetReader().Get(f.ctx, npcMindset.NPCMindsetID)

	require.NoError(t, err)
	require.Len(t, got.ActionWeights, len(matchaction.Actions()))
	for i, action := range matchaction.Actions() {
		require.Equal(t, string(action), got.ActionWeights[i].Action)
	}
	require.Equal(t, query.NPCMindsetActionWeight{Action: "end_turn", Weight: 7}, got.ActionWeights[len(got.ActionWeights)-1])
	require.Len(t, got.CriterionWeights, len(npcmindset.Criteria()))
	for i, criterion := range npcmindset.Criteria() {
		require.Equal(t, string(criterion), got.CriterionWeights[i].Criterion)
	}
	require.Equal(t, query.NPCMindsetCriterionWeight{Criterion: "magic_reserve", Weight: 3}, got.CriterionWeights[len(got.CriterionWeights)-1])
}

func TestNPCMindset_GetFailsForRowWithoutEveryWeight(t *testing.T) {
	f := newNPCMindsetFixture(t)
	missingAction := f.npcMindsetState("a", "A", readmodel.NPCMindsetStatusDraft)
	delete(missingAction.Metadata.ActionWeights, matchaction.ActionDrawCard)
	missingCriterion := f.npcMindsetState("b", "B", readmodel.NPCMindsetStatusDraft)
	delete(missingCriterion.Metadata.CriterionWeights, npcmindset.CriterionBattleHold)
	require.NoError(t, f.writer.Upsert(f.ctx, missingAction))
	require.NoError(t, f.writer.Upsert(f.ctx, missingCriterion))

	_, actionErr := repository.NewNPCMindsetReader().Get(f.ctx, missingAction.NPCMindsetID)
	_, criterionErr := repository.NewNPCMindsetReader().Get(f.ctx, missingCriterion.NPCMindsetID)

	require.Error(t, actionErr)
	require.Error(t, criterionErr)
}

func TestNPCMindset_GetFailsForMissingNPCMindset(t *testing.T) {
	f := newNPCMindsetFixture(t)

	_, err := repository.NewNPCMindsetReader().Get(f.ctx, uuid.New())

	require.ErrorIs(t, err, query.ErrNPCMindsetNotFound)
}

func TestNPCMindset_UpsertOverwritesSameNPCMindset(t *testing.T) {
	f := newNPCMindsetFixture(t)
	npcMindset := f.put(t, "fire", "Fire", readmodel.NPCMindsetStatusDraft)
	npcMindset.SortName = "water"
	npcMindset.Metadata.Name = "Water"
	npcMindset.Status = readmodel.NPCMindsetStatusRetired
	npcMindset.Metadata.Status = readmodel.NPCMindsetStatusRetired

	require.NoError(t, f.writer.Upsert(f.ctx, npcMindset))

	rows := f.listAll(t)
	require.Equal(t, []string{"Water"}, npcMindsetNames(rows))
	require.Equal(t, readmodel.NPCMindsetStatusRetired, rows[0].NPCMindset.Status)
}

func TestNPCMindset_DeleteRemovesNPCMindsetAndAcceptsMissingOne(t *testing.T) {
	f := newNPCMindsetFixture(t)
	deleted := f.put(t, "fire", "Fire", readmodel.NPCMindsetStatusDraft)
	f.put(t, "water", "Water", readmodel.NPCMindsetStatusDraft)

	require.NoError(t, f.writer.Delete(f.ctx, deleted))
	require.NoError(t, f.writer.Delete(f.ctx, deleted))

	require.Equal(t, []string{"Water"}, npcMindsetNames(f.listAll(t)))
}

func TestNPCMindset_ListsBySortNameWithCursor(t *testing.T) {
	f := newNPCMindsetFixture(t)
	f.put(t, "b", "B", readmodel.NPCMindsetStatusDraft)
	first := f.put(t, "a", "A", readmodel.NPCMindsetStatusDraft)
	f.put(t, "c", "C", readmodel.NPCMindsetStatusDraft)
	after := query.NPCMindsetListCursor{SortName: first.SortName, NPCMindsetID: first.NPCMindsetID}

	rows := f.list(t, query.ListNPCMindsetsCondition{After: &after, Limit: 1})

	require.Equal(t, []string{"A", "B", "C"}, npcMindsetNames(f.listAll(t)))
	require.Equal(t, []string{"B"}, npcMindsetNames(rows))
	require.Equal(t, query.NPCMindsetListCursor{SortName: "b", NPCMindsetID: rows[0].NPCMindset.ID}, rows[0].Cursor)
}

func TestNPCMindset_FiltersByStatuses(t *testing.T) {
	f := newNPCMindsetFixture(t)
	f.put(t, "a", "A", readmodel.NPCMindsetStatusDraft)
	f.put(t, "b", "B", readmodel.NPCMindsetStatusPublished)
	f.put(t, "c", "C", readmodel.NPCMindsetStatusRetired)

	published := f.list(t, query.ListNPCMindsetsCondition{Statuses: []readmodel.NPCMindsetStatus{readmodel.NPCMindsetStatusPublished}, Limit: 10})
	notDraft := f.list(t, query.ListNPCMindsetsCondition{
		Statuses: []readmodel.NPCMindsetStatus{readmodel.NPCMindsetStatusPublished, readmodel.NPCMindsetStatusRetired}, Limit: 10,
	})

	require.Equal(t, []string{"B"}, npcMindsetNames(published))
	require.Equal(t, []string{"B", "C"}, npcMindsetNames(notDraft))
}

// LIKE の記号（% _）は文字どおりに比べる（カードの一覧と同じ）
func TestNPCMindset_FiltersByNameContainsLiterally(t *testing.T) {
	f := newNPCMindsetFixture(t)
	for _, name := range []string{"50%off", "50xoff"} {
		f.put(t, name, name, readmodel.NPCMindsetStatusDraft)
	}

	rows := f.list(t, query.ListNPCMindsetsCondition{NameContains: "50%", Limit: 10})

	require.Equal(t, []string{"50%off"}, npcMindsetNames(rows))
}
