package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar/npcavatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck/npcdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

var (
	npcAvatarRegisteredAt = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	npcAvatarChangedAt    = time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
)

// 集約の Decide・Evolve でコマンドを順に当て、そのイベントを read model に当てる
type npcAvatarHistory struct {
	t      *testing.T
	state  npcavatar.State
	events []npcavatar.Event
}

func newNPCAvatarHistory(t *testing.T, publicID uuid.UUID, name string) *npcAvatarHistory {
	t.Helper()
	h := &npcAvatarHistory{t: t, state: npcavatar.InitialState()}
	h.apply(&npcavatar.RegisterCommand{ID: npcavatartest.ID(t, 1), PublicID: publicID, Name: npcavatartest.Name(t, name), RegisteredBy: operatortest.ID(t, 10)}, npcAvatarRegisteredAt)
	return h
}

func (h *npcAvatarHistory) apply(cmd npcavatar.Command, at time.Time) {
	h.t.Helper()
	events, err := npcavatar.Decide(h.state, cmd, at)
	require.NoError(h.t, err)
	for _, event := range events {
		h.state, err = npcavatar.Evolve(h.state, event)
		require.NoError(h.t, err)
	}
	h.events = append(h.events, events...)
}

func (h *npcAvatarHistory) project() readmodel.NPCAvatar {
	var state readmodel.NPCAvatar
	for _, event := range h.events {
		state = state.Evolve(event)
	}
	return state
}

func TestNPCAvatar_EvolvesFromRegistered(t *testing.T) {
	publicID := uuid.New()

	state := newNPCAvatarHistory(t, publicID, "Ｆire 剣士").project()

	require.Equal(t, publicID, state.NPCAvatarID)
	require.Equal(t, "fire 剣士", state.SortName)
	require.Equal(t, readmodel.NPCAvatarStatusDraft, state.Status)
	require.Equal(t, readmodel.NPCAvatarMetadata{
		ID: publicID, Name: "Ｆire 剣士", Status: readmodel.NPCAvatarStatusDraft, Revision: 1,
		RegisteredAt: npcAvatarRegisteredAt, UpdatedAt: npcAvatarRegisteredAt,
	}, state.Metadata)
	require.Equal(t, operatortest.ID(t, 10), state.RegisteredBy)
	require.Equal(t, operatortest.ID(t, 10), state.UpdatedBy)
	require.False(t, state.IsTerminal())
}

func TestNPCAvatar_RenameChangesNameSortNameAndUpdater(t *testing.T) {
	h := newNPCAvatarHistory(t, uuid.New(), "森の番人")
	h.apply(&npcavatar.RenameCommand{Name: npcavatartest.Name(t, "Ｓand"), RenamedBy: operatortest.ID(t, 11)}, npcAvatarChangedAt)

	state := h.project()

	require.Equal(t, "sand", state.SortName)
	require.Equal(t, "Ｓand", state.Metadata.Name)
	require.Equal(t, int64(2), state.Metadata.Revision)
	require.Equal(t, npcAvatarChangedAt, state.Metadata.UpdatedAt)
	require.Equal(t, operatortest.ID(t, 10), state.RegisteredBy)
	require.Equal(t, operatortest.ID(t, 11), state.UpdatedBy)
}

func TestNPCAvatar_DoesNotChangeTheGivenState(t *testing.T) {
	h := newNPCAvatarHistory(t, uuid.New(), "森の番人")
	before := h.project()
	h.apply(&npcavatar.RenameCommand{Name: npcavatartest.Name(t, "炎の剣士"), RenamedBy: operatortest.ID(t, 10)}, npcAvatarChangedAt)

	after := before.Evolve(h.events[len(h.events)-1])

	require.Equal(t, "森の番人", before.Metadata.Name)
	require.Equal(t, "炎の剣士", after.Metadata.Name)
}

func TestNPCAvatar_StatusEventsChangeStatusRevisionAndUpdater(t *testing.T) {
	h := newNPCAvatarHistory(t, uuid.New(), "森の番人")
	h.apply(&npcavatar.PublishCommand{PublishedBy: operatortest.ID(t, 11)}, npcAvatarChangedAt)
	published := h.project()
	h.apply(&npcavatar.RetireCommand{RetiredBy: operatortest.ID(t, 12)}, npcAvatarChangedAt)
	retired := h.project()

	require.Equal(t, readmodel.NPCAvatarStatusPublished, published.Status)
	require.Equal(t, readmodel.NPCAvatarStatusPublished, published.Metadata.Status)
	require.Equal(t, int64(2), published.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 11), published.UpdatedBy)
	require.Equal(t, readmodel.NPCAvatarStatusRetired, retired.Status)
	require.Equal(t, readmodel.NPCAvatarStatusRetired, retired.Metadata.Status)
	require.Equal(t, int64(3), retired.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 12), retired.UpdatedBy)
}

// 割り当てると割り当てた順に後ろへ足し、割り当てを外すと除く。どちらも版と更新した人・日時を変える
func TestNPCAvatar_DecksAssignedAndUnassignedChangeDecksRevisionAndUpdater(t *testing.T) {
	h := newNPCAvatarHistory(t, uuid.New(), "森の番人")
	h.apply(&npcavatar.SetDecksCommand{Decks: []npcdeck.ID{npcdecktest.ID(t, 41), npcdecktest.ID(t, 40)}, ChangedBy: operatortest.ID(t, 11)}, npcAvatarChangedAt)
	assigned := h.project()
	h.apply(&npcavatar.SetDecksCommand{Decks: []npcdeck.ID{npcdecktest.ID(t, 42)}, ChangedBy: operatortest.ID(t, 12)}, npcAvatarChangedAt)
	replaced := h.project()

	require.Equal(t, []npcdeck.ID{npcdecktest.ID(t, 41), npcdecktest.ID(t, 40)}, assigned.Decks)
	require.Equal(t, []npcdeck.ID{npcdecktest.ID(t, 42)}, replaced.Decks)
	// 外す（版 3）→ 割り当てる（版 4）の 2 つのイベント
	require.Equal(t, int64(4), replaced.Metadata.Revision)
	require.Equal(t, npcAvatarChangedAt, replaced.Metadata.UpdatedAt)
	require.Equal(t, operatortest.ID(t, 12), replaced.UpdatedBy)
	require.Equal(t, operatortest.ID(t, 10), replaced.RegisteredBy)
}

func TestNPCAvatar_DeletedEventMakesTerminal(t *testing.T) {
	h := newNPCAvatarHistory(t, uuid.New(), "森の番人")
	h.apply(&npcavatar.DeleteCommand{DeletedBy: operatortest.ID(t, 10)}, npcAvatarChangedAt)

	require.True(t, h.project().IsTerminal())
}

func TestNPCAvatar_IgnoresEventsOfOtherAggregates(t *testing.T) {
	state := readmodel.NPCAvatar{}.Evolve(registeredCreature(t, uuid.New(), "ドラゴン", npcAvatarRegisteredAt)[0])

	require.Equal(t, readmodel.NPCAvatar{}, state)
}

func TestNewNPCAvatarStatus_RejectsUnknownValue(t *testing.T) {
	_, err := readmodel.NewNPCAvatarStatus("archived")
	require.Error(t, err)
}
