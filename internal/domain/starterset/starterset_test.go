package starterset_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck/grantdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterset/startersettest"
)

var (
	registeredAt = time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	changedAt    = time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
)

type fixture struct {
	id         starterset.ID
	name       starterset.Name
	operatorID operator.ID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{
		id:         startersettest.ID(t, 30),
		name:       startersettest.Name(t, "スターターセット"),
		operatorID: operatortest.ID(t, 10),
	}
}

func (f fixture) registerCommand() *starterset.RegisterCommand {
	return &starterset.RegisterCommand{ID: f.id, PublicID: publicID, Name: f.name, RegisteredBy: f.operatorID}
}

func apply(t *testing.T, state starterset.State, cmd starterset.Command, now time.Time) (starterset.State, []starterset.Event) {
	t.Helper()
	events, err := starterset.Decide(state, cmd, now)
	require.NoError(t, err)
	for _, event := range events {
		state, err = starterset.Evolve(state, event)
		require.NoError(t, err)
	}
	return state, events
}

func (f fixture) register(t *testing.T) starterset.State {
	t.Helper()
	state, _ := apply(t, starterset.InitialState(), f.registerCommand(), registeredAt)
	return state
}

func (f fixture) setGrantDecksCommand(t *testing.T, values ...int64) *starterset.SetGrantDecksCommand {
	t.Helper()
	return &starterset.SetGrantDecksCommand{GrantDecks: grantDeckIDs(t, values...), ChangedBy: f.operatorID}
}

func (f fixture) setUnlockBoardsCommand(t *testing.T, values ...int64) *starterset.SetUnlockBoardsCommand {
	t.Helper()
	return &starterset.SetUnlockBoardsCommand{UnlockBoards: boardIDs(t, values...), ChangedBy: f.operatorID}
}

func grantDeckIDs(t *testing.T, values ...int64) []grantdeck.ID {
	t.Helper()
	ids := make([]grantdeck.ID, 0, len(values))
	for _, value := range values {
		ids = append(ids, grantdecktest.ID(t, value))
	}
	return ids
}

func boardIDs(t *testing.T, values ...int64) []board.ID {
	t.Helper()
	ids := make([]board.ID, 0, len(values))
	for _, value := range values {
		ids = append(ids, boardtest.ID(t, value))
	}
	return ids
}

func (f fixture) publishCommand() *starterset.PublishCommand {
	return &starterset.PublishCommand{PublishedBy: f.operatorID}
}

func (f fixture) retireCommand() *starterset.RetireCommand {
	return &starterset.RetireCommand{RetiredBy: f.operatorID}
}

func (f fixture) deleteCommand() *starterset.DeleteCommand {
	return &starterset.DeleteCommand{DeletedBy: f.operatorID}
}

// 公開できる最小の中身（配るデッキが 1 つ。ボードは 0 件）
func (f fixture) registeredWithGrantDeck(t *testing.T) starterset.State {
	t.Helper()
	state, _ := apply(t, f.register(t), f.setGrantDecksCommand(t, 40), changedAt)
	return state
}

func (f fixture) published(t *testing.T) starterset.State {
	t.Helper()
	state, _ := apply(t, f.registeredWithGrantDeck(t), f.publishCommand(), changedAt)
	return state
}

func (f fixture) retired(t *testing.T) starterset.State {
	t.Helper()
	state, _ := apply(t, f.published(t), f.retireCommand(), changedAt)
	return state
}

func TestDecide_RegisterMakesDraft(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, starterset.InitialState(), f.registerCommand(), registeredAt)

	require.Equal(t, []starterset.Event{&starterset.RegisteredEvent{
		EventMeta: starterset.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:  publicID, Name: f.name, RegisteredBy: f.operatorID,
	}}, events)
	require.Equal(t, starterset.StarterSet{
		ID: f.id, Version: 1, PublicID: publicID, Name: f.name, Status: starterset.StatusDraft,
	}, state)
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)

	_, err := starterset.Decide(f.register(t), f.registerCommand(), registeredAt)

	require.ErrorIs(t, err, starterset.ErrInvalidState)
}

func TestDecide_RenameChangesNameWithNextVersion(t *testing.T) {
	f := newFixture(t)
	newName := startersettest.Name(t, "新しいセット")

	state, events := apply(t, f.register(t), &starterset.RenameCommand{Name: newName, RenamedBy: f.operatorID}, changedAt)

	require.Equal(t, []starterset.Event{&starterset.RenamedEvent{
		EventMeta: starterset.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Name:      newName,
		RenamedBy: f.operatorID,
	}}, events)
	starterSet := state.(starterset.StarterSet)
	require.Equal(t, newName, starterSet.Name)
	require.Equal(t, int64(2), starterSet.Version)
}

func TestDecide_EmitsNothingWhenNothingChanges(t *testing.T) {
	f := newFixture(t)
	withLists := func(t *testing.T) starterset.State {
		t.Helper()
		state, _ := apply(t, f.register(t), f.setGrantDecksCommand(t, 40, 41), changedAt)
		state, _ = apply(t, state, f.setUnlockBoardsCommand(t, 50, 51), changedAt)
		return state
	}
	tests := []struct {
		name  string
		state starterset.State
		cmd   starterset.Command
	}{
		{name: "same_name", state: f.register(t), cmd: &starterset.RenameCommand{Name: f.name, RenamedBy: f.operatorID}},
		{name: "same_grant_decks", state: withLists(t), cmd: f.setGrantDecksCommand(t, 40, 41)},
		{name: "same_grant_decks_in_other_order", state: withLists(t), cmd: f.setGrantDecksCommand(t, 41, 40)},
		{name: "no_grant_decks_to_no_grant_decks", state: f.register(t), cmd: f.setGrantDecksCommand(t)},
		{name: "same_unlock_boards", state: withLists(t), cmd: f.setUnlockBoardsCommand(t, 50, 51)},
		{name: "same_unlock_boards_in_other_order", state: withLists(t), cmd: f.setUnlockBoardsCommand(t, 51, 50)},
		{name: "no_unlock_boards_to_no_unlock_boards", state: f.register(t), cmd: f.setUnlockBoardsCommand(t)},
		{name: "publish_published", state: f.published(t), cmd: f.publishCommand()},
		{name: "retire_retired", state: f.retired(t), cmd: f.retireCommand()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := starterset.Decide(tt.state, tt.cmd, changedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

// 今の一覧に無いものを割り当て、渡した一覧に無いものを外す。外す → 割り当てるの順に出し、版はイベントの数だけ進む。
// 一覧の順は割り当てた順（残したものの後ろに、新しく割り当てたものが付く）
func TestDecide_SetGrantDecksUnassignsMissingAndAssignsNewDecks(t *testing.T) {
	f := newFixture(t)
	before, _ := apply(t, f.register(t), f.setGrantDecksCommand(t, 40, 41), changedAt)

	state, events := apply(t, before, f.setGrantDecksCommand(t, 42, 40), changedAt)

	require.Equal(t, []starterset.Event{
		&starterset.GrantDecksUnassignedEvent{
			EventMeta:    starterset.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
			GrantDecks:   grantDeckIDs(t, 41),
			UnassignedBy: f.operatorID,
		},
		&starterset.GrantDecksAssignedEvent{
			EventMeta:  starterset.EventMeta{EntityID: f.id, AggregateVersion: 4, OccurredAt: changedAt},
			GrantDecks: grantDeckIDs(t, 42),
			AssignedBy: f.operatorID,
		},
	}, events)
	starterSet := state.(starterset.StarterSet)
	require.Equal(t, grantDeckIDs(t, 40, 42), starterSet.GrantDecks)
	require.Equal(t, int64(4), starterSet.Version)
}

func TestDecide_SetGrantDecksOnlyAssignsWhenNothingIsRemoved(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.registeredWithGrantDeck(t), f.setGrantDecksCommand(t, 40, 41), changedAt)

	require.Equal(t, []starterset.Event{&starterset.GrantDecksAssignedEvent{
		EventMeta:  starterset.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		GrantDecks: grantDeckIDs(t, 41),
		AssignedBy: f.operatorID,
	}}, events)
	require.Equal(t, grantDeckIDs(t, 40, 41), state.(starterset.StarterSet).GrantDecks)
}

func TestDecide_SetUnlockBoardsUnassignsMissingAndAssignsNewBoards(t *testing.T) {
	f := newFixture(t)
	before, _ := apply(t, f.register(t), f.setUnlockBoardsCommand(t, 50, 51), changedAt)

	state, events := apply(t, before, f.setUnlockBoardsCommand(t, 52, 50), changedAt)

	require.Equal(t, []starterset.Event{
		&starterset.UnlockBoardsUnassignedEvent{
			EventMeta:    starterset.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
			UnlockBoards: boardIDs(t, 51),
			UnassignedBy: f.operatorID,
		},
		&starterset.UnlockBoardsAssignedEvent{
			EventMeta:    starterset.EventMeta{EntityID: f.id, AggregateVersion: 4, OccurredAt: changedAt},
			UnlockBoards: boardIDs(t, 52),
			AssignedBy:   f.operatorID,
		},
	}, events)
	require.Equal(t, boardIDs(t, 50, 52), state.(starterset.StarterSet).UnlockBoards)
}

func TestDecide_SetListsCanClearThem(t *testing.T) {
	f := newFixture(t)
	withDecks, _ := apply(t, f.register(t), f.setGrantDecksCommand(t, 40, 41), changedAt)
	withBoards, _ := apply(t, f.register(t), f.setUnlockBoardsCommand(t, 50), changedAt)

	decksCleared, deckEvents := apply(t, withDecks, f.setGrantDecksCommand(t), changedAt)
	boardsCleared, boardEvents := apply(t, withBoards, f.setUnlockBoardsCommand(t), changedAt)

	require.Len(t, deckEvents, 1)
	require.Empty(t, decksCleared.(starterset.StarterSet).GrantDecks)
	require.Len(t, boardEvents, 1)
	require.Empty(t, boardsCleared.(starterset.StarterSet).UnlockBoards)
}

func TestDecide_PublishMakesPublishedWithGrantDecksAndNoBoards(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.registeredWithGrantDeck(t), f.publishCommand(), changedAt)

	require.Equal(t, []starterset.Event{&starterset.PublishedEvent{
		EventMeta:   starterset.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		PublishedBy: f.operatorID,
	}}, events)
	require.Equal(t, starterset.StatusPublished, state.(starterset.StarterSet).Status)
}

// 配布は 0 個のデッキでは始められない（startergrant.ErrInvalidDeckCount）。ボードは 0 件でもよい
func TestDecide_RejectsPublishWithoutGrantDecks(t *testing.T) {
	f := newFixture(t)
	onlyBoards, _ := apply(t, f.register(t), f.setUnlockBoardsCommand(t, 50), changedAt)

	_, withoutAnything := starterset.Decide(f.register(t), f.publishCommand(), changedAt)
	_, withBoardsOnly := starterset.Decide(onlyBoards, f.publishCommand(), changedAt)

	require.ErrorIs(t, withoutAnything, starterset.ErrNoGrantDecks)
	require.ErrorIs(t, withBoardsOnly, starterset.ErrNoGrantDecks)
}

func TestDecide_RetiredCanBePublishedAgain(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.retired(t), f.publishCommand(), changedAt)

	require.Len(t, events, 1)
	require.Equal(t, starterset.StatusPublished, state.(starterset.StarterSet).Status)
}

func TestDecide_RetireMakesRetiredOnlyFromPublished(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.published(t), f.retireCommand(), changedAt)

	require.Equal(t, []starterset.Event{&starterset.RetiredEvent{
		EventMeta: starterset.EventMeta{EntityID: f.id, AggregateVersion: 4, OccurredAt: changedAt},
		RetiredBy: f.operatorID,
	}}, events)
	require.Equal(t, starterset.StatusRetired, state.(starterset.StarterSet).Status)

	_, err := starterset.Decide(f.register(t), f.retireCommand(), changedAt)
	require.ErrorIs(t, err, starterset.ErrInvalidStatusTransition)
}

// 配っている間に中身が変わらないよう、準備中以外では変えられず、消せない。準備中に戻す操作も無い
func TestDecide_RejectsChangesAndDeleteUnlessDraft(t *testing.T) {
	f := newFixture(t)
	commands := []struct {
		name string
		cmd  starterset.Command
	}{
		{name: "rename", cmd: &starterset.RenameCommand{Name: startersettest.Name(t, "新しいセット"), RenamedBy: f.operatorID}},
		{name: "set_grant_decks", cmd: f.setGrantDecksCommand(t, 40)},
		{name: "set_unlock_boards", cmd: f.setUnlockBoardsCommand(t, 50)},
		{name: "delete", cmd: f.deleteCommand()},
	}
	states := []struct {
		name  string
		state starterset.State
	}{
		{name: "published", state: f.published(t)},
		{name: "retired", state: f.retired(t)},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := starterset.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, starterset.ErrNotDraft)
			})
		}
	}
}

func TestDecide_DeleteMakesDeletedStarterSet(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.register(t), f.deleteCommand(), changedAt)

	require.Equal(t, []starterset.Event{&starterset.DeletedEvent{
		EventMeta: starterset.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		DeletedBy: f.operatorID,
	}}, events)
	require.Equal(t, starterset.DeletedStarterSet{ID: f.id, Version: 2}, state)
}

func TestDecide_RejectsChangesUnlessRegistered(t *testing.T) {
	f := newFixture(t)
	deleted, _ := apply(t, f.register(t), f.deleteCommand(), changedAt)
	commands := []struct {
		name string
		cmd  starterset.Command
	}{
		{name: "rename", cmd: &starterset.RenameCommand{Name: startersettest.Name(t, "新しいセット"), RenamedBy: f.operatorID}},
		{name: "set_grant_decks", cmd: f.setGrantDecksCommand(t, 40)},
		{name: "set_unlock_boards", cmd: f.setUnlockBoardsCommand(t, 50)},
		{name: "publish", cmd: f.publishCommand()},
		{name: "retire", cmd: f.retireCommand()},
		{name: "delete", cmd: f.deleteCommand()},
	}
	states := []struct {
		name  string
		state starterset.State
	}{
		{name: "unregistered", state: starterset.InitialState()},
		{name: "deleted", state: deleted},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := starterset.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, starterset.ErrInvalidState)
			})
		}
	}
}

func TestStatus_TransitionsFollowRules(t *testing.T) {
	tests := []struct {
		name        string
		from        starterset.Status
		publishTo   starterset.Status
		publishDiff bool
		retireTo    starterset.Status
		retireDiff  bool
		retireErr   error
	}{
		{name: "draft", from: starterset.StatusDraft, publishTo: starterset.StatusPublished, publishDiff: true, retireTo: starterset.StatusDraft, retireErr: starterset.ErrInvalidStatusTransition},
		{name: "published", from: starterset.StatusPublished, publishTo: starterset.StatusPublished, retireTo: starterset.StatusRetired, retireDiff: true},
		{name: "retired", from: starterset.StatusRetired, publishTo: starterset.StatusPublished, publishDiff: true, retireTo: starterset.StatusRetired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			to, changed, err := tt.from.Publish()
			require.NoError(t, err)
			require.Equal(t, tt.publishTo, to)
			require.Equal(t, tt.publishDiff, changed)

			to, changed, err = tt.from.Retire()
			require.ErrorIs(t, err, tt.retireErr)
			require.Equal(t, tt.retireTo, to)
			require.Equal(t, tt.retireDiff, changed)
		})
	}
}

func TestNewStatus_RejectsUnknownValue(t *testing.T) {
	_, err := starterset.NewStatus("archived")

	require.ErrorIs(t, err, starterset.ErrStatusUnknown)
}

func TestNewName_RejectsBlankAndTooLong(t *testing.T) {
	name, err := starterset.NewName("  スターターセット  ")
	require.NoError(t, err)
	require.Equal(t, "スターターセット", name.String())
	_, err = starterset.NewName(strings.Repeat("あ", starterset.MaxNameLength))
	require.NoError(t, err)

	_, err = starterset.NewName("   ")
	require.ErrorIs(t, err, starterset.ErrNameEmpty)
	_, err = starterset.NewName(strings.Repeat("あ", starterset.MaxNameLength+1))
	require.ErrorIs(t, err, starterset.ErrNameTooLong)
}

// 公開中の名前だけでなく、状態によらず、すべてのスターターセットで比べる。比べ方はカード名と同じ
func TestNameKey_TreatsWidthAndCaseAsSame(t *testing.T) {
	require.Equal(t,
		starterset.NameKey(startersettest.Name(t, "Ｓet A")),
		starterset.NameKey(startersettest.Name(t, "set a")))
}

// 公開中は同時に 1 つだけ。上限はゲーム全体で 1 つ数える（持ち主はゲーム全体のルールの集約）
func TestPublishedQuota_CountsOnceForTheGame(t *testing.T) {
	rules, err := gamerules.NewID(5)
	require.NoError(t, err)

	quota := starterset.PublishedQuota(rules)

	require.Equal(t, starterset.PublishedQuotaType, quota.Type())
	require.Equal(t, int64(5), quota.OwnerID())
	require.Equal(t, 1, starterset.MaxPublished)
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	f := newFixture(t)
	meta := starterset.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt}
	events := []starterset.Event{
		&starterset.RegisteredEvent{EventMeta: meta, PublicID: publicID, Name: f.name, RegisteredBy: f.operatorID},
		&starterset.RenamedEvent{EventMeta: meta, Name: startersettest.Name(t, "新しいスターターセット"), RenamedBy: f.operatorID},
		&starterset.GrantDecksAssignedEvent{EventMeta: meta, GrantDecks: grantDeckIDs(t, 40, 41), AssignedBy: f.operatorID},
		&starterset.GrantDecksUnassignedEvent{EventMeta: meta, GrantDecks: grantDeckIDs(t, 40), UnassignedBy: f.operatorID},
		&starterset.UnlockBoardsAssignedEvent{EventMeta: meta, UnlockBoards: boardIDs(t, 50, 51), AssignedBy: f.operatorID},
		&starterset.UnlockBoardsUnassignedEvent{EventMeta: meta, UnlockBoards: boardIDs(t, 50), UnassignedBy: f.operatorID},
		&starterset.PublishedEvent{EventMeta: meta, PublishedBy: f.operatorID},
		&starterset.RetiredEvent{EventMeta: meta, RetiredBy: f.operatorID},
		&starterset.DeletedEvent{EventMeta: meta, DeletedBy: f.operatorID},
	}
	for _, event := range events {
		t.Run(event.EventType().String(), func(t *testing.T) {
			require.Equal(t, 1, event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := starterset.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, event, restored)
		})
	}
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := starterset.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(starterset.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}
