package npcavatar_test

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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar/npcavatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck/npcdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset/npcmindsettest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
)

var (
	registeredAt = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	changedAt    = time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
)

type fixture struct {
	id         npcavatar.ID
	name       npcavatar.Name
	operatorID operator.ID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{
		id:         npcavatartest.ID(t, 30),
		name:       npcavatartest.Name(t, "見習いの魔術師"),
		operatorID: operatortest.ID(t, 10),
	}
}

func (f fixture) registerCommand() *npcavatar.RegisterCommand {
	return &npcavatar.RegisterCommand{ID: f.id, PublicID: publicID, Name: f.name, RegisteredBy: f.operatorID}
}

func apply(t *testing.T, state npcavatar.State, cmd npcavatar.Command, now time.Time) (npcavatar.State, []npcavatar.Event) {
	t.Helper()
	events, err := npcavatar.Decide(state, cmd, now)
	require.NoError(t, err)
	for _, event := range events {
		state, err = npcavatar.Evolve(state, event)
		require.NoError(t, err)
	}
	return state, events
}

func (f fixture) register(t *testing.T) npcavatar.State {
	t.Helper()
	state, _ := apply(t, npcavatar.InitialState(), f.registerCommand(), registeredAt)
	return state
}

func (f fixture) renameCommand(t *testing.T, name string) *npcavatar.RenameCommand {
	t.Helper()
	return &npcavatar.RenameCommand{Name: npcavatartest.Name(t, name), RenamedBy: f.operatorID}
}

func (f fixture) setDecksCommand(t *testing.T, decks ...int64) *npcavatar.SetDecksCommand {
	t.Helper()
	return &npcavatar.SetDecksCommand{Decks: deckIDs(t, decks...), ChangedBy: f.operatorID}
}

func (f fixture) setMindsetsCommand(t *testing.T, mindsets ...int64) *npcavatar.SetMindsetsCommand {
	t.Helper()
	return &npcavatar.SetMindsetsCommand{Mindsets: mindsetIDs(t, mindsets...), ChangedBy: f.operatorID}
}

func mindsetIDs(t *testing.T, values ...int64) []npcmindset.ID {
	t.Helper()
	ids := make([]npcmindset.ID, 0, len(values))
	for _, value := range values {
		ids = append(ids, npcmindsettest.ID(t, value))
	}
	return ids
}

func deckIDs(t *testing.T, values ...int64) []npcdeck.ID {
	t.Helper()
	ids := make([]npcdeck.ID, 0, len(values))
	for _, value := range values {
		ids = append(ids, npcdecktest.ID(t, value))
	}
	return ids
}

func (f fixture) publishCommand() *npcavatar.PublishCommand {
	return &npcavatar.PublishCommand{PublishedBy: f.operatorID}
}

func (f fixture) retireCommand() *npcavatar.RetireCommand {
	return &npcavatar.RetireCommand{RetiredBy: f.operatorID}
}

func (f fixture) deleteCommand() *npcavatar.DeleteCommand {
	return &npcavatar.DeleteCommand{DeletedBy: f.operatorID}
}

func (f fixture) published(t *testing.T) npcavatar.State {
	t.Helper()
	state, _ := apply(t, f.register(t), f.publishCommand(), changedAt)
	return state
}

func (f fixture) retired(t *testing.T) npcavatar.State {
	t.Helper()
	state, _ := apply(t, f.published(t), f.retireCommand(), changedAt)
	return state
}

func TestDecide_RegisterMakesDraft(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, npcavatar.InitialState(), f.registerCommand(), registeredAt)

	require.Equal(t, []npcavatar.Event{&npcavatar.RegisteredEvent{
		EventMeta: npcavatar.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:  publicID, Name: f.name, RegisteredBy: f.operatorID,
	}}, events)
	require.Equal(t, npcavatar.NPCAvatar{
		ID: f.id, Version: 1, PublicID: publicID, Name: f.name, Status: npcavatar.StatusDraft,
	}, state)
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)

	_, err := npcavatar.Decide(f.register(t), f.registerCommand(), registeredAt)

	require.ErrorIs(t, err, npcavatar.ErrInvalidState)
}

func TestDecide_RenameChangesNameWithNextVersion(t *testing.T) {
	f := newFixture(t)
	newName := npcavatartest.Name(t, "森の番人")

	state, events := apply(t, f.register(t), &npcavatar.RenameCommand{Name: newName, RenamedBy: f.operatorID}, changedAt)

	require.Equal(t, []npcavatar.Event{&npcavatar.RenamedEvent{
		EventMeta: npcavatar.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Name:      newName,
		RenamedBy: f.operatorID,
	}}, events)
	npcAvatar := state.(npcavatar.NPCAvatar)
	require.Equal(t, newName, npcAvatar.Name)
	require.Equal(t, int64(2), npcAvatar.Version)
}

func TestDecide_EmitsNothingWhenNothingChanges(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name  string
		state npcavatar.State
		cmd   npcavatar.Command
	}{
		{name: "same_name", state: f.register(t), cmd: &npcavatar.RenameCommand{Name: f.name, RenamedBy: f.operatorID}},
		{name: "same_decks", state: withDecks(t, f, 40, 41), cmd: f.setDecksCommand(t, 40, 41)},
		{name: "same_decks_in_other_order", state: withDecks(t, f, 40, 41), cmd: f.setDecksCommand(t, 41, 40)},
		{name: "no_decks_to_no_decks", state: f.register(t), cmd: f.setDecksCommand(t)},
		{name: "same_mindsets", state: withMindsets(t, f, 60, 61), cmd: f.setMindsetsCommand(t, 60, 61)},
		{name: "same_mindsets_in_other_order", state: withMindsets(t, f, 60, 61), cmd: f.setMindsetsCommand(t, 61, 60)},
		{name: "no_mindsets_to_no_mindsets", state: f.register(t), cmd: f.setMindsetsCommand(t)},
		{name: "publish_published", state: f.published(t), cmd: f.publishCommand()},
		{name: "retire_retired", state: f.retired(t), cmd: f.retireCommand()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := npcavatar.Decide(tt.state, tt.cmd, changedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

func withDecks(t *testing.T, f fixture, decks ...int64) npcavatar.State {
	t.Helper()
	state, _ := apply(t, f.register(t), f.setDecksCommand(t, decks...), changedAt)
	return state
}

// 今の一覧に無いものを割り当て、渡した一覧に無いものを外す。外す → 割り当てるの順に出し、版はイベントの数だけ進む。
// 一覧の順は割り当てた順（残したものの後ろに、新しく割り当てたものが付く）
func TestDecide_SetDecksUnassignsMissingAndAssignsNewDecks(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, withDecks(t, f, 40, 41), f.setDecksCommand(t, 42, 40), changedAt)

	require.Equal(t, []npcavatar.Event{
		&npcavatar.DecksUnassignedEvent{
			EventMeta:    npcavatar.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
			Decks:        deckIDs(t, 41),
			UnassignedBy: f.operatorID,
		},
		&npcavatar.DecksAssignedEvent{
			EventMeta:  npcavatar.EventMeta{EntityID: f.id, AggregateVersion: 4, OccurredAt: changedAt},
			Decks:      deckIDs(t, 42),
			AssignedBy: f.operatorID,
		},
	}, events)
	npcAvatar := state.(npcavatar.NPCAvatar)
	require.Equal(t, deckIDs(t, 40, 42), npcAvatar.Decks)
	require.Equal(t, int64(4), npcAvatar.Version)
}

func TestDecide_SetDecksOnlyAssignsWhenNothingIsRemoved(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, withDecks(t, f, 40), f.setDecksCommand(t, 40, 41), changedAt)

	require.Equal(t, []npcavatar.Event{&npcavatar.DecksAssignedEvent{
		EventMeta:  npcavatar.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Decks:      deckIDs(t, 41),
		AssignedBy: f.operatorID,
	}}, events)
	require.Equal(t, deckIDs(t, 40, 41), state.(npcavatar.NPCAvatar).Decks)
}

func TestDecide_SetDecksCanClearList(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, withDecks(t, f, 40, 41), f.setDecksCommand(t), changedAt)

	require.Equal(t, []npcavatar.Event{&npcavatar.DecksUnassignedEvent{
		EventMeta:    npcavatar.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Decks:        deckIDs(t, 40, 41),
		UnassignedBy: f.operatorID,
	}}, events)
	require.Empty(t, state.(npcavatar.NPCAvatar).Decks)
}

func withMindsets(t *testing.T, f fixture, mindsets ...int64) npcavatar.State {
	t.Helper()
	state, _ := apply(t, f.register(t), f.setMindsetsCommand(t, mindsets...), changedAt)
	return state
}

// 割り当て方は SetDecks と同じ。外す → 割り当てるの順に出し、版はイベントの数だけ進む。デッキの割り当てとは独立している
func TestDecide_SetMindsetsUnassignsMissingAndAssignsNewMindsets(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, withMindsets(t, f, 60, 61), f.setMindsetsCommand(t, 62, 60), changedAt)

	require.Equal(t, []npcavatar.Event{
		&npcavatar.MindsetsUnassignedEvent{
			EventMeta:    npcavatar.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
			Mindsets:     mindsetIDs(t, 61),
			UnassignedBy: f.operatorID,
		},
		&npcavatar.MindsetsAssignedEvent{
			EventMeta:  npcavatar.EventMeta{EntityID: f.id, AggregateVersion: 4, OccurredAt: changedAt},
			Mindsets:   mindsetIDs(t, 62),
			AssignedBy: f.operatorID,
		},
	}, events)
	npcAvatar := state.(npcavatar.NPCAvatar)
	require.Equal(t, mindsetIDs(t, 60, 62), npcAvatar.Mindsets)
	require.Empty(t, npcAvatar.Decks)
	require.Equal(t, int64(4), npcAvatar.Version)
}

func TestDecide_SetMindsetsOnlyAssignsWhenNothingIsRemoved(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, withMindsets(t, f, 60), f.setMindsetsCommand(t, 60, 61), changedAt)

	require.Equal(t, []npcavatar.Event{&npcavatar.MindsetsAssignedEvent{
		EventMeta:  npcavatar.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Mindsets:   mindsetIDs(t, 61),
		AssignedBy: f.operatorID,
	}}, events)
	require.Equal(t, mindsetIDs(t, 60, 61), state.(npcavatar.NPCAvatar).Mindsets)
}

func TestDecide_SetMindsetsCanClearList(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, withMindsets(t, f, 60, 61), f.setMindsetsCommand(t), changedAt)

	require.Equal(t, []npcavatar.Event{&npcavatar.MindsetsUnassignedEvent{
		EventMeta:    npcavatar.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Mindsets:     mindsetIDs(t, 60, 61),
		UnassignedBy: f.operatorID,
	}}, events)
	require.Empty(t, state.(npcavatar.NPCAvatar).Mindsets)
}

func TestDecide_PublishMakesPublished(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.register(t), f.publishCommand(), changedAt)

	require.Equal(t, []npcavatar.Event{&npcavatar.PublishedEvent{
		EventMeta:   npcavatar.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		PublishedBy: f.operatorID,
	}}, events)
	require.Equal(t, npcavatar.StatusPublished, state.(npcavatar.NPCAvatar).Status)
}

func TestDecide_RetiredCanBePublishedAgain(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.retired(t), f.publishCommand(), changedAt)

	require.Len(t, events, 1)
	require.Equal(t, npcavatar.StatusPublished, state.(npcavatar.NPCAvatar).Status)
}

func TestDecide_RetireMakesRetiredOnlyFromPublished(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.published(t), f.retireCommand(), changedAt)

	require.Equal(t, []npcavatar.Event{&npcavatar.RetiredEvent{
		EventMeta: npcavatar.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		RetiredBy: f.operatorID,
	}}, events)
	require.Equal(t, npcavatar.StatusRetired, state.(npcavatar.NPCAvatar).Status)

	_, err := npcavatar.Decide(f.register(t), f.retireCommand(), changedAt)
	require.ErrorIs(t, err, npcavatar.ErrInvalidStatusTransition)
}

// 使われている間に名前・使うデッキと思考パターンが変わらないよう、準備中以外では変えられず、消せない。準備中に戻す操作も無い
func TestDecide_RejectsRenameAndDeleteUnlessDraft(t *testing.T) {
	f := newFixture(t)
	commands := []struct {
		name string
		cmd  npcavatar.Command
	}{
		{name: "rename", cmd: f.renameCommand(t, "森の番人")},
		{name: "set_decks", cmd: f.setDecksCommand(t, 40)},
		{name: "set_mindsets", cmd: f.setMindsetsCommand(t, 60)},
		{name: "delete", cmd: f.deleteCommand()},
	}
	states := []struct {
		name  string
		state npcavatar.State
	}{
		{name: "published", state: f.published(t)},
		{name: "retired", state: f.retired(t)},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := npcavatar.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, npcavatar.ErrNotDraft)
			})
		}
	}
}

func TestDecide_DeleteMakesDeletedNPCAvatar(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.register(t), f.deleteCommand(), changedAt)

	require.Equal(t, []npcavatar.Event{&npcavatar.DeletedEvent{
		EventMeta: npcavatar.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		DeletedBy: f.operatorID,
	}}, events)
	require.Equal(t, npcavatar.DeletedNPCAvatar{ID: f.id, Version: 2}, state)
}

func TestDecide_RejectsChangesUnlessRegistered(t *testing.T) {
	f := newFixture(t)
	deleted, _ := apply(t, f.register(t), f.deleteCommand(), changedAt)
	commands := []struct {
		name string
		cmd  npcavatar.Command
	}{
		{name: "rename", cmd: f.renameCommand(t, "森の番人")},
		{name: "set_decks", cmd: f.setDecksCommand(t, 40)},
		{name: "set_mindsets", cmd: f.setMindsetsCommand(t, 60)},
		{name: "publish", cmd: f.publishCommand()},
		{name: "retire", cmd: f.retireCommand()},
		{name: "delete", cmd: f.deleteCommand()},
	}
	states := []struct {
		name  string
		state npcavatar.State
	}{
		{name: "unregistered", state: npcavatar.InitialState()},
		{name: "deleted", state: deleted},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := npcavatar.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, npcavatar.ErrInvalidState)
			})
		}
	}
}

func TestStatus_TransitionsFollowRules(t *testing.T) {
	tests := []struct {
		name        string
		from        npcavatar.Status
		publishTo   npcavatar.Status
		publishDiff bool
		retireTo    npcavatar.Status
		retireDiff  bool
		retireErr   error
	}{
		{name: "draft", from: npcavatar.StatusDraft, publishTo: npcavatar.StatusPublished, publishDiff: true, retireTo: npcavatar.StatusDraft, retireErr: npcavatar.ErrInvalidStatusTransition},
		{name: "published", from: npcavatar.StatusPublished, publishTo: npcavatar.StatusPublished, retireTo: npcavatar.StatusRetired, retireDiff: true},
		{name: "retired", from: npcavatar.StatusRetired, publishTo: npcavatar.StatusPublished, publishDiff: true, retireTo: npcavatar.StatusRetired},
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
	_, err := npcavatar.NewStatus("archived")

	require.ErrorIs(t, err, npcavatar.ErrStatusUnknown)
}

func TestNewName_RejectsBlankAndTooLong(t *testing.T) {
	name, err := npcavatar.NewName("  森の番人  ")
	require.NoError(t, err)
	require.Equal(t, "森の番人", name.String())
	_, err = npcavatar.NewName(strings.Repeat("あ", npcavatar.MaxNameLength))
	require.NoError(t, err)

	_, err = npcavatar.NewName("   ")
	require.ErrorIs(t, err, npcavatar.ErrNameEmpty)
	_, err = npcavatar.NewName(strings.Repeat("あ", npcavatar.MaxNameLength+1))
	require.ErrorIs(t, err, npcavatar.ErrNameTooLong)
}

// 公開中の名前だけでなく、状態によらず、すべての NPC のアバターで比べる。比べ方はカード名と同じ
func TestNameKey_TreatsWidthAndCaseAsSame(t *testing.T) {
	require.Equal(t,
		npcavatar.NameKey(npcavatartest.Name(t, "Ｆire 剣士")),
		npcavatar.NameKey(npcavatartest.Name(t, "fire 剣士")))
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	f := newFixture(t)
	meta := npcavatar.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt}
	events := []npcavatar.Event{
		&npcavatar.RegisteredEvent{EventMeta: meta, PublicID: publicID, Name: f.name, RegisteredBy: f.operatorID},
		&npcavatar.RenamedEvent{EventMeta: meta, Name: npcavatartest.Name(t, "森の番人"), RenamedBy: f.operatorID},
		&npcavatar.DecksAssignedEvent{EventMeta: meta, Decks: deckIDs(t, 40, 41), AssignedBy: f.operatorID},
		&npcavatar.DecksUnassignedEvent{EventMeta: meta, Decks: deckIDs(t, 40), UnassignedBy: f.operatorID},
		&npcavatar.MindsetsAssignedEvent{EventMeta: meta, Mindsets: mindsetIDs(t, 60, 61), AssignedBy: f.operatorID},
		&npcavatar.MindsetsUnassignedEvent{EventMeta: meta, Mindsets: mindsetIDs(t, 60), UnassignedBy: f.operatorID},
		&npcavatar.PublishedEvent{EventMeta: meta, PublishedBy: f.operatorID},
		&npcavatar.RetiredEvent{EventMeta: meta, RetiredBy: f.operatorID},
		&npcavatar.DeletedEvent{EventMeta: meta, DeletedBy: f.operatorID},
	}
	for _, event := range events {
		t.Run(event.EventType().String(), func(t *testing.T) {
			require.Equal(t, 1, event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := npcavatar.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, event, restored)
		})
	}
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := npcavatar.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(npcavatar.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}
