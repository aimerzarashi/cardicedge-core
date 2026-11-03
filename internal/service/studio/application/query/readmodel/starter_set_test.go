package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck/grantdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterset/startersettest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

var (
	starterSetRegisteredAt = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	starterSetChangedAt    = time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
)

// 集約の Decide・Evolve でコマンドを順に当て、そのイベントを read model に当てる
type starterSetHistory struct {
	t      *testing.T
	state  starterset.State
	events []starterset.Event
}

func newStarterSetHistory(t *testing.T, publicID uuid.UUID, name string) *starterSetHistory {
	t.Helper()
	h := &starterSetHistory{t: t, state: starterset.InitialState()}
	h.apply(&starterset.RegisterCommand{ID: startersettest.ID(t, 1), PublicID: publicID, Name: startersettest.Name(t, name), RegisteredBy: operatortest.ID(t, 10)}, starterSetRegisteredAt)
	return h
}

func (h *starterSetHistory) apply(cmd starterset.Command, at time.Time) {
	h.t.Helper()
	events, err := starterset.Decide(h.state, cmd, at)
	require.NoError(h.t, err)
	for _, event := range events {
		h.state, err = starterset.Evolve(h.state, event)
		require.NoError(h.t, err)
	}
	h.events = append(h.events, events...)
}

func (h *starterSetHistory) project() readmodel.StarterSet {
	var state readmodel.StarterSet
	for _, event := range h.events {
		state = state.Evolve(event)
	}
	return state
}

func TestStarterSet_EvolvesFromRegistered(t *testing.T) {
	publicID := uuid.New()

	state := newStarterSetHistory(t, publicID, "Ｆire 剣士").project()

	require.Equal(t, publicID, state.StarterSetID)
	require.Equal(t, "fire 剣士", state.SortName)
	require.Equal(t, readmodel.StarterSetStatusDraft, state.Status)
	require.Equal(t, readmodel.StarterSetMetadata{
		ID: publicID, Name: "Ｆire 剣士", Status: readmodel.StarterSetStatusDraft, Revision: 1,
		RegisteredAt: starterSetRegisteredAt, UpdatedAt: starterSetRegisteredAt,
	}, state.Metadata)
	require.Equal(t, operatortest.ID(t, 10), state.RegisteredBy)
	require.Equal(t, operatortest.ID(t, 10), state.UpdatedBy)
	require.False(t, state.IsTerminal())
}

func TestStarterSet_RenameChangesNameSortNameAndUpdater(t *testing.T) {
	h := newStarterSetHistory(t, uuid.New(), "森の番人")
	h.apply(&starterset.RenameCommand{Name: startersettest.Name(t, "Ｓand"), RenamedBy: operatortest.ID(t, 11)}, starterSetChangedAt)

	state := h.project()

	require.Equal(t, "sand", state.SortName)
	require.Equal(t, "Ｓand", state.Metadata.Name)
	require.Equal(t, int64(2), state.Metadata.Revision)
	require.Equal(t, starterSetChangedAt, state.Metadata.UpdatedAt)
	require.Equal(t, operatortest.ID(t, 10), state.RegisteredBy)
	require.Equal(t, operatortest.ID(t, 11), state.UpdatedBy)
}

func TestStarterSet_DoesNotChangeTheGivenState(t *testing.T) {
	h := newStarterSetHistory(t, uuid.New(), "森の番人")
	before := h.project()
	h.apply(&starterset.RenameCommand{Name: startersettest.Name(t, "炎の剣士"), RenamedBy: operatortest.ID(t, 10)}, starterSetChangedAt)

	after := before.Evolve(h.events[len(h.events)-1])

	require.Equal(t, "森の番人", before.Metadata.Name)
	require.Equal(t, "炎の剣士", after.Metadata.Name)
}

func TestStarterSet_StatusEventsChangeStatusRevisionAndUpdater(t *testing.T) {
	h := newStarterSetHistory(t, uuid.New(), "森の番人")
	// 公開するには配るデッキが 1 つ以上要る（版 2）
	h.apply(&starterset.SetGrantDecksCommand{GrantDecks: []grantdeck.ID{grantdecktest.ID(t, 40)}, ChangedBy: operatortest.ID(t, 10)}, starterSetChangedAt)
	h.apply(&starterset.PublishCommand{PublishedBy: operatortest.ID(t, 11)}, starterSetChangedAt)
	published := h.project()
	h.apply(&starterset.RetireCommand{RetiredBy: operatortest.ID(t, 12)}, starterSetChangedAt)
	retired := h.project()

	require.Equal(t, readmodel.StarterSetStatusPublished, published.Status)
	require.Equal(t, readmodel.StarterSetStatusPublished, published.Metadata.Status)
	require.Equal(t, int64(3), published.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 11), published.UpdatedBy)
	require.Equal(t, readmodel.StarterSetStatusRetired, retired.Status)
	require.Equal(t, readmodel.StarterSetStatusRetired, retired.Metadata.Status)
	require.Equal(t, int64(4), retired.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 12), retired.UpdatedBy)
}

// 割り当てると割り当てた順に後ろへ足し、割り当てを外すと除く。どちらも版と更新した人・日時を変える
func TestStarterSet_GrantDecksAssignedAndUnassignedChangeGrantDecksRevisionAndUpdater(t *testing.T) {
	h := newStarterSetHistory(t, uuid.New(), "標準")
	h.apply(&starterset.SetGrantDecksCommand{GrantDecks: []grantdeck.ID{grantdecktest.ID(t, 41), grantdecktest.ID(t, 40)}, ChangedBy: operatortest.ID(t, 11)}, starterSetChangedAt)
	assigned := h.project()
	h.apply(&starterset.SetGrantDecksCommand{GrantDecks: []grantdeck.ID{grantdecktest.ID(t, 42)}, ChangedBy: operatortest.ID(t, 12)}, starterSetChangedAt)
	replaced := h.project()

	require.Equal(t, []grantdeck.ID{grantdecktest.ID(t, 41), grantdecktest.ID(t, 40)}, assigned.GrantDecks)
	require.Equal(t, []grantdeck.ID{grantdecktest.ID(t, 42)}, replaced.GrantDecks)
	// 外す（版 3）→ 割り当てる（版 4）の 2 つのイベント
	require.Equal(t, int64(4), replaced.Metadata.Revision)
	require.Equal(t, starterSetChangedAt, replaced.Metadata.UpdatedAt)
	require.Equal(t, operatortest.ID(t, 12), replaced.UpdatedBy)
	require.Equal(t, operatortest.ID(t, 10), replaced.RegisteredBy)
}

func TestStarterSet_UnlockBoardsAssignedAndUnassignedChangeUnlockBoardsRevisionAndUpdater(t *testing.T) {
	h := newStarterSetHistory(t, uuid.New(), "標準")
	h.apply(&starterset.SetUnlockBoardsCommand{UnlockBoards: []board.ID{boardtest.ID(t, 51), boardtest.ID(t, 50)}, ChangedBy: operatortest.ID(t, 11)}, starterSetChangedAt)
	assigned := h.project()
	h.apply(&starterset.SetUnlockBoardsCommand{UnlockBoards: []board.ID{boardtest.ID(t, 52)}, ChangedBy: operatortest.ID(t, 12)}, starterSetChangedAt)
	replaced := h.project()

	require.Equal(t, []board.ID{boardtest.ID(t, 51), boardtest.ID(t, 50)}, assigned.UnlockBoards)
	require.Equal(t, []board.ID{boardtest.ID(t, 52)}, replaced.UnlockBoards)
	require.Empty(t, replaced.GrantDecks)
	require.Equal(t, int64(4), replaced.Metadata.Revision)
	require.Equal(t, operatortest.ID(t, 12), replaced.UpdatedBy)
}

func TestStarterSet_DeletedEventMakesTerminal(t *testing.T) {
	h := newStarterSetHistory(t, uuid.New(), "森の番人")
	h.apply(&starterset.DeleteCommand{DeletedBy: operatortest.ID(t, 10)}, starterSetChangedAt)

	require.True(t, h.project().IsTerminal())
}

func TestStarterSet_IgnoresEventsOfOtherAggregates(t *testing.T) {
	state := readmodel.StarterSet{}.Evolve(registeredCreature(t, uuid.New(), "ドラゴン", starterSetRegisteredAt)[0])

	require.Equal(t, readmodel.StarterSet{}, state)
}

func TestNewStarterSetStatus_RejectsUnknownValue(t *testing.T) {
	_, err := readmodel.NewStarterSetStatus("archived")
	require.Error(t, err)
}
