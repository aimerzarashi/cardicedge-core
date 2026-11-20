package npcmindset_test

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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/matchaction"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset/npcmindsettest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
)

var (
	registeredAt = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	changedAt    = time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
)

type fixture struct {
	id         npcmindset.ID
	name       npcmindset.Name
	operatorID operator.ID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{
		id:         npcmindsettest.ID(t, 60),
		name:       npcmindsettest.Name(t, "バランス"),
		operatorID: operatortest.ID(t, 10),
	}
}

func (f fixture) registerCommand() *npcmindset.RegisterCommand {
	return &npcmindset.RegisterCommand{ID: f.id, PublicID: publicID, Name: f.name, RegisteredBy: f.operatorID}
}

func apply(t *testing.T, state npcmindset.State, cmd npcmindset.Command, now time.Time) (npcmindset.State, []npcmindset.Event) {
	t.Helper()
	events, err := npcmindset.Decide(state, cmd, now)
	require.NoError(t, err)
	for _, event := range events {
		state, err = npcmindset.Evolve(state, event)
		require.NoError(t, err)
	}
	return state, events
}

func (f fixture) register(t *testing.T) npcmindset.State {
	t.Helper()
	state, _ := apply(t, npcmindset.InitialState(), f.registerCommand(), registeredAt)
	return state
}

func (f fixture) setActionWeightsCommand(weights npcmindset.ActionWeights) *npcmindset.SetActionWeightsCommand {
	return &npcmindset.SetActionWeightsCommand{Weights: weights, ChangedBy: f.operatorID}
}

func (f fixture) setCriterionWeightsCommand(weights npcmindset.CriterionWeights) *npcmindset.SetCriterionWeightsCommand {
	return &npcmindset.SetCriterionWeightsCommand{Weights: weights, ChangedBy: f.operatorID}
}

func (f fixture) publishCommand() *npcmindset.PublishCommand {
	return &npcmindset.PublishCommand{PublishedBy: f.operatorID}
}

func (f fixture) retireCommand() *npcmindset.RetireCommand {
	return &npcmindset.RetireCommand{RetiredBy: f.operatorID}
}

func (f fixture) deleteCommand() *npcmindset.DeleteCommand {
	return &npcmindset.DeleteCommand{DeletedBy: f.operatorID}
}

func (f fixture) published(t *testing.T) npcmindset.State {
	t.Helper()
	state, _ := apply(t, f.register(t), f.publishCommand(), changedAt)
	return state
}

func (f fixture) retired(t *testing.T) npcmindset.State {
	t.Helper()
	state, _ := apply(t, f.published(t), f.retireCommand(), changedAt)
	return state
}

// 保存済みのイベントを読み直すときは確かめない（NewAction は知らない種類を作らせない）ので、知らない種類・基準は JSON から読んで作る
func unknownAction(t *testing.T) matchaction.Action {
	t.Helper()
	weights := npcmindsettest.ActionWeights(t, nil)
	require.NoError(t, json.Unmarshal([]byte(`{"unknown":1}`), &weights))
	for action := range weights {
		return action
	}
	require.FailNow(t, "no action")
	return ""
}

func unknownCriterion(t *testing.T) npcmindset.Criterion {
	t.Helper()
	weights := npcmindsettest.CriterionWeights(t, nil)
	require.NoError(t, json.Unmarshal([]byte(`{"unknown":1}`), &weights))
	for criterion := range weights {
		return criterion
	}
	require.FailNow(t, "no criterion")
	return ""
}

// 重みを変えた、種類のすべてを持つ重み
func aggressiveActionWeights(t *testing.T) npcmindset.ActionWeights {
	t.Helper()
	weights := npcmindset.DefaultActionWeights()
	weights[matchaction.ActionInvadeTerritory] = npcmindsettest.Weight(t, 100)
	weights[matchaction.ActionPayToll] = npcmindsettest.Weight(t, 0)
	return weights
}

func aggressiveCriterionWeights(t *testing.T) npcmindset.CriterionWeights {
	t.Helper()
	weights := npcmindset.DefaultCriterionWeights()
	weights[npcmindset.CriterionOwnTotalMagic] = npcmindsettest.Weight(t, 80)
	return weights
}

// 既定の重みを後から変えても、過去の登録のイベントから作る状態が変わらないよう、登録のイベントが既定の重みを持つ
func TestDecide_RegisterMakesDraftWithDefaultWeightsOfOne(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, npcmindset.InitialState(), f.registerCommand(), registeredAt)

	require.Equal(t, []npcmindset.Event{&npcmindset.RegisteredEvent{
		EventMeta:        npcmindset.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:         publicID,
		Name:             f.name,
		ActionWeights:    npcmindset.DefaultActionWeights(),
		CriterionWeights: npcmindset.DefaultCriterionWeights(),
		RegisteredBy:     f.operatorID,
	}}, events)
	require.Equal(t, npcmindset.NPCMindset{
		ID: f.id, Version: 1, PublicID: publicID, Name: f.name, Status: npcmindset.StatusDraft,
		ActionWeights: npcmindset.DefaultActionWeights(), CriterionWeights: npcmindset.DefaultCriterionWeights(),
	}, state)
}

func TestDefaultWeights_HaveEveryActionAndCriterionWithOne(t *testing.T) {
	actions := npcmindset.DefaultActionWeights()
	criteria := npcmindset.DefaultCriterionWeights()

	require.Len(t, actions, len(matchaction.Actions()))
	for _, action := range matchaction.Actions() {
		require.Equal(t, 1, actions[action].Int())
	}
	require.Len(t, criteria, len(npcmindset.Criteria()))
	for _, criterion := range npcmindset.Criteria() {
		require.Equal(t, 1, criteria[criterion].Int())
	}
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)

	_, err := npcmindset.Decide(f.register(t), f.registerCommand(), registeredAt)

	require.ErrorIs(t, err, npcmindset.ErrInvalidState)
}

func TestDecide_RenameChangesNameWithNextVersion(t *testing.T) {
	f := newFixture(t)
	newName := npcmindsettest.Name(t, "侵略型")

	state, events := apply(t, f.register(t), &npcmindset.RenameCommand{Name: newName, RenamedBy: f.operatorID}, changedAt)

	require.Equal(t, []npcmindset.Event{&npcmindset.RenamedEvent{
		EventMeta: npcmindset.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Name:      newName,
		RenamedBy: f.operatorID,
	}}, events)
	mindset := state.(npcmindset.NPCMindset)
	require.Equal(t, newName, mindset.Name)
	require.Equal(t, int64(2), mindset.Version)
}

func TestDecide_SetActionWeightsReplacesAllWeights(t *testing.T) {
	f := newFixture(t)
	weights := aggressiveActionWeights(t)

	state, events := apply(t, f.register(t), f.setActionWeightsCommand(weights), changedAt)

	require.Equal(t, []npcmindset.Event{&npcmindset.ActionWeightsChangedEvent{
		EventMeta: npcmindset.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Weights:   weights,
		ChangedBy: f.operatorID,
	}}, events)
	mindset := state.(npcmindset.NPCMindset)
	require.Equal(t, weights, mindset.ActionWeights)
	require.Equal(t, npcmindset.DefaultCriterionWeights(), mindset.CriterionWeights)
	require.Equal(t, int64(2), mindset.Version)
}

func TestDecide_SetCriterionWeightsReplacesAllWeights(t *testing.T) {
	f := newFixture(t)
	weights := aggressiveCriterionWeights(t)

	state, events := apply(t, f.register(t), f.setCriterionWeightsCommand(weights), changedAt)

	require.Equal(t, []npcmindset.Event{&npcmindset.CriterionWeightsChangedEvent{
		EventMeta: npcmindset.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Weights:   weights,
		ChangedBy: f.operatorID,
	}}, events)
	mindset := state.(npcmindset.NPCMindset)
	require.Equal(t, weights, mindset.CriterionWeights)
	require.Equal(t, npcmindset.DefaultActionWeights(), mindset.ActionWeights)
}

// 版を進めないため（画面で見た版を古くしないため）
func TestDecide_EmitsNothingWhenNothingChanges(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name  string
		state npcmindset.State
		cmd   npcmindset.Command
	}{
		{name: "same_name", state: f.register(t), cmd: &npcmindset.RenameCommand{Name: f.name, RenamedBy: f.operatorID}},
		{name: "same_action_weights", state: f.register(t), cmd: f.setActionWeightsCommand(npcmindset.DefaultActionWeights())},
		{name: "same_criterion_weights", state: f.register(t), cmd: f.setCriterionWeightsCommand(npcmindset.DefaultCriterionWeights())},
		{name: "publish_published", state: f.published(t), cmd: f.publishCommand()},
		{name: "retire_retired", state: f.retired(t), cmd: f.retireCommand()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := npcmindset.Decide(tt.state, tt.cmd, changedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

func TestDecide_EmitsNothingWhenSameWeightsAreSetAgainAfterChange(t *testing.T) {
	f := newFixture(t)
	state, _ := apply(t, f.register(t), f.setActionWeightsCommand(aggressiveActionWeights(t)), changedAt)

	events, err := npcmindset.Decide(state, f.setActionWeightsCommand(aggressiveActionWeights(t)), changedAt)

	require.NoError(t, err)
	require.Empty(t, events)
}

func TestDecide_RejectsActionWeightsThatDoNotMatchTheActions(t *testing.T) {
	f := newFixture(t)
	missing := npcmindset.DefaultActionWeights()
	delete(missing, matchaction.ActionEndTurn)
	extra := npcmindset.DefaultActionWeights()
	extra[unknownAction(t)] = npcmindsettest.Weight(t, 1)
	replaced := npcmindset.DefaultActionWeights()
	delete(replaced, matchaction.ActionEndTurn)
	replaced[unknownAction(t)] = npcmindsettest.Weight(t, 1)
	tests := []struct {
		name    string
		weights npcmindset.ActionWeights
	}{
		{name: "empty", weights: npcmindsettest.ActionWeights(t, nil)},
		{name: "nil", weights: nil},
		{name: "missing_action", weights: missing},
		{name: "unknown_action", weights: extra},
		{name: "unknown_action_in_place_of_known", weights: replaced},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := npcmindset.Decide(f.register(t), f.setActionWeightsCommand(tt.weights), changedAt)

			require.ErrorIs(t, err, npcmindset.ErrActionWeightsMismatch)
		})
	}
}

func TestDecide_RejectsCriterionWeightsThatDoNotMatchTheCriteria(t *testing.T) {
	f := newFixture(t)
	missing := npcmindset.DefaultCriterionWeights()
	delete(missing, npcmindset.CriterionRivalTotalMagic)
	extra := npcmindset.DefaultCriterionWeights()
	extra[unknownCriterion(t)] = npcmindsettest.Weight(t, 1)
	replaced := npcmindset.DefaultCriterionWeights()
	delete(replaced, npcmindset.CriterionRivalTotalMagic)
	replaced[unknownCriterion(t)] = npcmindsettest.Weight(t, 1)
	tests := []struct {
		name    string
		weights npcmindset.CriterionWeights
	}{
		{name: "empty", weights: npcmindsettest.CriterionWeights(t, nil)},
		{name: "nil", weights: nil},
		{name: "missing_criterion", weights: missing},
		{name: "unknown_criterion", weights: extra},
		{name: "unknown_criterion_in_place_of_known", weights: replaced},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := npcmindset.Decide(f.register(t), f.setCriterionWeightsCommand(tt.weights), changedAt)

			require.ErrorIs(t, err, npcmindset.ErrCriterionWeightsMismatch)
		})
	}
}

func TestNewWeight_AcceptsZeroToOneHundredOnly(t *testing.T) {
	tests := []struct {
		name  string
		value int
		want  error
	}{
		{name: "negative", value: -1, want: npcmindset.ErrWeightOutOfRange},
		{name: "zero", value: 0},
		{name: "one", value: 1},
		{name: "upper_limit", value: 100},
		{name: "over_upper_limit", value: 101, want: npcmindset.ErrWeightOutOfRange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weight, err := npcmindset.NewWeight(tt.value)

			require.ErrorIs(t, err, tt.want)
			if tt.want == nil {
				require.Equal(t, tt.value, weight.Int())
			}
		})
	}
}

// 状態を書き換えないよう、イベントが持つ重みの map と状態の map は別にする
func TestEvolve_DoesNotShareWeightsWithTheEventOrTheGivenState(t *testing.T) {
	f := newFixture(t)
	before := f.register(t)
	weights := aggressiveActionWeights(t)
	event := &npcmindset.ActionWeightsChangedEvent{EventMeta: npcmindset.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt}, Weights: weights, ChangedBy: f.operatorID}

	after, err := npcmindset.Evolve(before, event)
	require.NoError(t, err)
	weights[matchaction.ActionEndTurn] = npcmindsettest.Weight(t, 77)

	require.Equal(t, npcmindset.DefaultActionWeights(), before.(npcmindset.NPCMindset).ActionWeights)
	require.Equal(t, 1, after.(npcmindset.NPCMindset).ActionWeights[matchaction.ActionEndTurn].Int())
}

func TestDecide_PublishMakesPublished(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.register(t), f.publishCommand(), changedAt)

	require.Equal(t, []npcmindset.Event{&npcmindset.PublishedEvent{
		EventMeta:   npcmindset.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		PublishedBy: f.operatorID,
	}}, events)
	require.Equal(t, npcmindset.StatusPublished, state.(npcmindset.NPCMindset).Status)
}

func TestDecide_RetiredCanBePublishedAgain(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.retired(t), f.publishCommand(), changedAt)

	require.Len(t, events, 1)
	require.Equal(t, npcmindset.StatusPublished, state.(npcmindset.NPCMindset).Status)
}

func TestDecide_RetireMakesRetiredOnlyFromPublished(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.published(t), f.retireCommand(), changedAt)

	require.Equal(t, []npcmindset.Event{&npcmindset.RetiredEvent{
		EventMeta: npcmindset.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		RetiredBy: f.operatorID,
	}}, events)
	require.Equal(t, npcmindset.StatusRetired, state.(npcmindset.NPCMindset).Status)

	_, err := npcmindset.Decide(f.register(t), f.retireCommand(), changedAt)
	require.ErrorIs(t, err, npcmindset.ErrInvalidStatusTransition)
}

// 使われている間に中身が変わらないよう、準備中以外では名前・重みを変えられず、消せない。準備中に戻す操作も無い
func TestDecide_RejectsEditAndDeleteUnlessDraft(t *testing.T) {
	f := newFixture(t)
	commands := []struct {
		name string
		cmd  npcmindset.Command
	}{
		{name: "rename", cmd: &npcmindset.RenameCommand{Name: npcmindsettest.Name(t, "侵略型"), RenamedBy: f.operatorID}},
		{name: "set_action_weights", cmd: f.setActionWeightsCommand(aggressiveActionWeights(t))},
		{name: "set_criterion_weights", cmd: f.setCriterionWeightsCommand(aggressiveCriterionWeights(t))},
		{name: "delete", cmd: f.deleteCommand()},
	}
	states := []struct {
		name  string
		state npcmindset.State
	}{
		{name: "published", state: f.published(t)},
		{name: "retired", state: f.retired(t)},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := npcmindset.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, npcmindset.ErrNotDraft)
			})
		}
	}
}

func TestDecide_DeleteMakesDeletedNPCMindset(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.register(t), f.deleteCommand(), changedAt)

	require.Equal(t, []npcmindset.Event{&npcmindset.DeletedEvent{
		EventMeta: npcmindset.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		DeletedBy: f.operatorID,
	}}, events)
	require.Equal(t, npcmindset.DeletedNPCMindset{ID: f.id, Version: 2}, state)
}

func TestDecide_RejectsChangesUnlessRegistered(t *testing.T) {
	f := newFixture(t)
	deleted, _ := apply(t, f.register(t), f.deleteCommand(), changedAt)
	commands := []struct {
		name string
		cmd  npcmindset.Command
	}{
		{name: "rename", cmd: &npcmindset.RenameCommand{Name: npcmindsettest.Name(t, "侵略型"), RenamedBy: f.operatorID}},
		{name: "set_action_weights", cmd: f.setActionWeightsCommand(aggressiveActionWeights(t))},
		{name: "set_criterion_weights", cmd: f.setCriterionWeightsCommand(aggressiveCriterionWeights(t))},
		{name: "publish", cmd: f.publishCommand()},
		{name: "retire", cmd: f.retireCommand()},
		{name: "delete", cmd: f.deleteCommand()},
	}
	states := []struct {
		name  string
		state npcmindset.State
	}{
		{name: "unregistered", state: npcmindset.InitialState()},
		{name: "deleted", state: deleted},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := npcmindset.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, npcmindset.ErrInvalidState)
			})
		}
	}
}

func TestStatus_TransitionsFollowRules(t *testing.T) {
	tests := []struct {
		name        string
		from        npcmindset.Status
		publishTo   npcmindset.Status
		publishDiff bool
		retireTo    npcmindset.Status
		retireDiff  bool
		retireErr   error
	}{
		{name: "draft", from: npcmindset.StatusDraft, publishTo: npcmindset.StatusPublished, publishDiff: true, retireTo: npcmindset.StatusDraft, retireErr: npcmindset.ErrInvalidStatusTransition},
		{name: "published", from: npcmindset.StatusPublished, publishTo: npcmindset.StatusPublished, retireTo: npcmindset.StatusRetired, retireDiff: true},
		{name: "retired", from: npcmindset.StatusRetired, publishTo: npcmindset.StatusPublished, publishDiff: true, retireTo: npcmindset.StatusRetired},
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
	_, err := npcmindset.NewStatus("archived")

	require.ErrorIs(t, err, npcmindset.ErrStatusUnknown)
}

func TestNewName_RejectsBlankAndTooLong(t *testing.T) {
	name, err := npcmindset.NewName("  育成型  ")
	require.NoError(t, err)
	require.Equal(t, "育成型", name.String())

	_, err = npcmindset.NewName(strings.Repeat("あ", npcmindset.MaxNameLength))
	require.NoError(t, err)

	_, err = npcmindset.NewName("   ")
	require.ErrorIs(t, err, npcmindset.ErrNameEmpty)
	_, err = npcmindset.NewName(strings.Repeat("あ", npcmindset.MaxNameLength+1))
	require.ErrorIs(t, err, npcmindset.ErrNameTooLong)
}

// 公開中の名前だけでなく、状態によらず、すべての思考パターンで比べる。比べ方はカード名と同じ
func TestNameKey_TreatsWidthAndCaseAsSame(t *testing.T) {
	require.Equal(t,
		npcmindset.NameKey(npcmindsettest.Name(t, "Ｆire 型")),
		npcmindset.NameKey(npcmindsettest.Name(t, "fire 型")))
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	f := newFixture(t)
	meta := npcmindset.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt}
	events := []npcmindset.Event{
		&npcmindset.RegisteredEvent{
			EventMeta: meta, PublicID: publicID, Name: f.name,
			ActionWeights: npcmindset.DefaultActionWeights(), CriterionWeights: npcmindset.DefaultCriterionWeights(), RegisteredBy: f.operatorID,
		},
		&npcmindset.RenamedEvent{EventMeta: meta, Name: npcmindsettest.Name(t, "侵略型"), RenamedBy: f.operatorID},
		&npcmindset.ActionWeightsChangedEvent{EventMeta: meta, Weights: aggressiveActionWeights(t), ChangedBy: f.operatorID},
		&npcmindset.CriterionWeightsChangedEvent{EventMeta: meta, Weights: aggressiveCriterionWeights(t), ChangedBy: f.operatorID},
		&npcmindset.PublishedEvent{EventMeta: meta, PublishedBy: f.operatorID},
		&npcmindset.RetiredEvent{EventMeta: meta, RetiredBy: f.operatorID},
		&npcmindset.DeletedEvent{EventMeta: meta, DeletedBy: f.operatorID},
	}
	for _, event := range events {
		t.Run(event.EventType().String(), func(t *testing.T) {
			require.Equal(t, 1, event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := npcmindset.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, event, restored)
		})
	}
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := npcmindset.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(npcmindset.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}

func TestNewCriterion_RejectsUnknownValue(t *testing.T) {
	criterion, err := npcmindset.NewCriterion("rival_total_magic")
	require.NoError(t, err)
	require.Equal(t, npcmindset.CriterionRivalTotalMagic, criterion)

	_, err = npcmindset.NewCriterion("unknown")
	require.ErrorIs(t, err, npcmindset.ErrCriterionUnknown)
}
