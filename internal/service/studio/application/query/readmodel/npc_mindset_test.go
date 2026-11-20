package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/matchaction"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset/npcmindsettest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

var (
	npcMindsetRegisteredAt = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	npcMindsetChangedAt    = time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
)

// 集約の Decide・Evolve でコマンドを順に当て、そのイベントを read model に当てる
type npcMindsetHistory struct {
	t      *testing.T
	state  npcmindset.State
	events []npcmindset.Event
}

func newNPCMindsetHistory(t *testing.T, publicID uuid.UUID, name string) *npcMindsetHistory {
	t.Helper()
	h := &npcMindsetHistory{t: t, state: npcmindset.InitialState()}
	h.apply(&npcmindset.RegisterCommand{ID: npcmindsettest.ID(t, 1), PublicID: publicID, Name: npcmindsettest.Name(t, name), RegisteredBy: operatortest.ID(t, 10)}, npcMindsetRegisteredAt)
	return h
}

func (h *npcMindsetHistory) apply(cmd npcmindset.Command, at time.Time) {
	h.t.Helper()
	events, err := npcmindset.Decide(h.state, cmd, at)
	require.NoError(h.t, err)
	for _, event := range events {
		h.state, err = npcmindset.Evolve(h.state, event)
		require.NoError(h.t, err)
	}
	h.events = append(h.events, events...)
}

func (h *npcMindsetHistory) project() readmodel.NPCMindset {
	var state readmodel.NPCMindset
	for _, event := range h.events {
		state = state.Evolve(event)
	}
	return state
}

func TestNPCMindset_EvolvesFromRegistered(t *testing.T) {
	publicID := uuid.New()

	state := newNPCMindsetHistory(t, publicID, "Ｆire 剣士").project()

	require.Equal(t, publicID, state.NPCMindsetID)
	require.Equal(t, "fire 剣士", state.SortName)
	require.Equal(t, readmodel.NPCMindsetStatusDraft, state.Status)
	require.Equal(t, readmodel.NPCMindsetMetadata{
		ID: publicID, Name: "Ｆire 剣士", Status: readmodel.NPCMindsetStatusDraft, Revision: 1,
		RegisteredAt: npcMindsetRegisteredAt, UpdatedAt: npcMindsetRegisteredAt,
		ActionWeights: actionWeightValues(npcmindset.DefaultActionWeights()), CriterionWeights: criterionWeightValues(npcmindset.DefaultCriterionWeights()),
	}, state.Metadata)
	require.Equal(t, operatortest.ID(t, 10), state.RegisteredBy)
	require.Equal(t, operatortest.ID(t, 10), state.UpdatedBy)
	require.False(t, state.IsTerminal())
}

func TestNPCMindset_RenameChangesNameSortNameAndUpdater(t *testing.T) {
	h := newNPCMindsetHistory(t, uuid.New(), "森の番人")
	h.apply(&npcmindset.RenameCommand{Name: npcmindsettest.Name(t, "Ｓand"), RenamedBy: operatortest.ID(t, 11)}, npcMindsetChangedAt)

	state := h.project()

	require.Equal(t, "sand", state.SortName)
	require.Equal(t, "Ｓand", state.Metadata.Name)
	require.Equal(t, int64(2), state.Metadata.Revision)
	require.Equal(t, npcMindsetChangedAt, state.Metadata.UpdatedAt)
	require.Equal(t, operatortest.ID(t, 10), state.RegisteredBy)
	require.Equal(t, operatortest.ID(t, 11), state.UpdatedBy)
}

func TestNPCMindset_DoesNotChangeTheGivenState(t *testing.T) {
	h := newNPCMindsetHistory(t, uuid.New(), "森の番人")
	before := h.project()
	h.apply(&npcmindset.RenameCommand{Name: npcmindsettest.Name(t, "炎の剣士"), RenamedBy: operatortest.ID(t, 10)}, npcMindsetChangedAt)

	after := before.Evolve(h.events[len(h.events)-1])

	require.Equal(t, "森の番人", before.Metadata.Name)
	require.Equal(t, "炎の剣士", after.Metadata.Name)
}

func TestNPCMindset_StatusEventsChangeStatusRevisionAndUpdater(t *testing.T) {
	h := newNPCMindsetHistory(t, uuid.New(), "森の番人")
	h.apply(&npcmindset.PublishCommand{PublishedBy: operatortest.ID(t, 11)}, npcMindsetChangedAt)
	published := h.project()
	h.apply(&npcmindset.RetireCommand{RetiredBy: operatortest.ID(t, 12)}, npcMindsetChangedAt)
	retired := h.project()

	require.Equal(t, readmodel.NPCMindsetStatusPublished, published.Status)
	require.Equal(t, readmodel.NPCMindsetStatusPublished, published.Metadata.Status)
	require.Equal(t, int64(2), published.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 11), published.UpdatedBy)
	require.Equal(t, readmodel.NPCMindsetStatusRetired, retired.Status)
	require.Equal(t, readmodel.NPCMindsetStatusRetired, retired.Metadata.Status)
	require.Equal(t, int64(3), retired.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 12), retired.UpdatedBy)
}

// 登録のときの重みは domain の既定の値（すべて 1）で、種類・基準のすべてを持つ
func TestNPCMindset_RegisteredHasEveryWeightOfOne(t *testing.T) {
	state := newNPCMindsetHistory(t, uuid.New(), "バランス").project()

	require.Len(t, state.Metadata.ActionWeights, len(matchaction.Actions()))
	for _, action := range matchaction.Actions() {
		require.Equal(t, 1, state.Metadata.ActionWeights[action])
	}
	require.Len(t, state.Metadata.CriterionWeights, len(npcmindset.Criteria()))
	for _, criterion := range npcmindset.Criteria() {
		require.Equal(t, 1, state.Metadata.CriterionWeights[criterion])
	}
}

// 重みの変更は、変えた後の全体で置き換える。版と更新した人・日時を変え、変えなかった側の重みは残る
func TestNPCMindset_WeightsChangedReplaceWeightsRevisionAndUpdater(t *testing.T) {
	h := newNPCMindsetHistory(t, uuid.New(), "侵略型")
	actionWeights := npcmindset.DefaultActionWeights()
	actionWeights[matchaction.ActionInvadeTerritory] = npcmindsettest.Weight(t, 100)
	criterionWeights := npcmindset.DefaultCriterionWeights()
	criterionWeights[npcmindset.CriterionRivalTotalMagic] = npcmindsettest.Weight(t, 3)
	h.apply(&npcmindset.SetActionWeightsCommand{Weights: actionWeights, ChangedBy: operatortest.ID(t, 11)}, npcMindsetChangedAt)
	h.apply(&npcmindset.SetCriterionWeightsCommand{Weights: criterionWeights, ChangedBy: operatortest.ID(t, 12)}, npcMindsetChangedAt)

	state := h.project()

	require.Equal(t, 100, state.Metadata.ActionWeights[matchaction.ActionInvadeTerritory])
	require.Equal(t, 1, state.Metadata.ActionWeights[matchaction.ActionPayToll])
	require.Equal(t, 3, state.Metadata.CriterionWeights[npcmindset.CriterionRivalTotalMagic])
	require.Equal(t, 1, state.Metadata.CriterionWeights[npcmindset.CriterionTerritoryCount])
	require.Equal(t, int64(3), state.Metadata.Revision)
	require.Equal(t, npcMindsetChangedAt, state.Metadata.UpdatedAt)
	require.Equal(t, operatortest.ID(t, 12), state.UpdatedBy)
	require.Equal(t, operatortest.ID(t, 10), state.RegisteredBy)
}

func TestNPCMindset_DeletedEventMakesTerminal(t *testing.T) {
	h := newNPCMindsetHistory(t, uuid.New(), "森の番人")
	h.apply(&npcmindset.DeleteCommand{DeletedBy: operatortest.ID(t, 10)}, npcMindsetChangedAt)

	require.True(t, h.project().IsTerminal())
}

func TestNPCMindset_IgnoresEventsOfOtherAggregates(t *testing.T) {
	state := readmodel.NPCMindset{}.Evolve(registeredCreature(t, uuid.New(), "ドラゴン", npcMindsetRegisteredAt)[0])

	require.Equal(t, readmodel.NPCMindset{}, state)
}

func TestNewNPCMindsetStatus_RejectsUnknownValue(t *testing.T) {
	_, err := readmodel.NewNPCMindsetStatus("archived")
	require.Error(t, err)
}

func actionWeightValues(weights npcmindset.ActionWeights) map[matchaction.Action]int {
	values := map[matchaction.Action]int{}
	for action, weight := range weights {
		values[action] = weight.Int()
	}
	return values
}

func criterionWeightValues(weights npcmindset.CriterionWeights) map[npcmindset.Criterion]int {
	values := map[npcmindset.Criterion]int{}
	for criterion, weight := range weights {
		values[criterion] = weight.Int()
	}
	return values
}
