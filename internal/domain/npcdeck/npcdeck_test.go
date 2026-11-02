package npcdeck_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck/npcdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck/presetdecktest"
)

var (
	registeredAt = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	changedAt    = time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
	presetID     = uuid.MustParse("66666666-7777-8888-9999-000000000000")
)

type fixture struct {
	id         npcdeck.ID
	name       presetdeck.Name
	operatorID operator.ID
	dragon     presetdeck.CardRef
	potion     presetdeck.CardRef
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{
		id:         npcdecktest.ID(t, 30),
		name:       presetdecktest.Name(t, "炎のデッキ"),
		operatorID: operatortest.ID(t, 10),
		dragon:     presetdecktest.CardRef(t, creature.Types().EntityType, 20),
		potion:     presetdecktest.CardRef(t, item.Types().EntityType, 21),
	}
}

func (f fixture) registerCommand() *npcdeck.RegisterCommand {
	return &npcdeck.RegisterCommand{ID: f.id, PublicID: publicID, Name: f.name, RegisteredBy: f.operatorID}
}

func apply(t *testing.T, state npcdeck.State, cmd npcdeck.Command, now time.Time) (npcdeck.State, []npcdeck.Event) {
	t.Helper()
	events, err := npcdeck.Decide(state, cmd, now)
	require.NoError(t, err)
	for _, event := range events {
		state, err = npcdeck.Evolve(state, event)
		require.NoError(t, err)
	}
	return state, events
}

func (f fixture) register(t *testing.T) npcdeck.State {
	t.Helper()
	state, _ := apply(t, npcdeck.InitialState(), f.registerCommand(), registeredAt)
	return state
}

func (f fixture) setCountsCommand(t *testing.T, counts map[presetdeck.CardRef]int) *npcdeck.SetCardCountsCommand {
	t.Helper()
	cardCounts := make(map[presetdeck.CardRef]presetdeck.CardCount, len(counts))
	for card, count := range counts {
		cardCounts[card] = presetdecktest.CardCount(t, count)
	}
	return &npcdeck.SetCardCountsCommand{Counts: cardCounts, Rules: gamerules.Defaults().Deck, ChangedBy: f.operatorID}
}

func (f fixture) setCounts(t *testing.T, state npcdeck.State, counts map[presetdeck.CardRef]int) npcdeck.State {
	t.Helper()
	state, _ = apply(t, state, f.setCountsCommand(t, counts), changedAt)
	return state
}

// 公開できる、合計がちょうど 40 枚の準備中
func (f fixture) fullDraft(t *testing.T) npcdeck.State {
	t.Helper()
	return f.setCounts(t, f.register(t), map[presetdeck.CardRef]int{f.dragon: 4, f.potion: 4})
}

func (f fixture) publishCommand(rules gamerules.Deck) *npcdeck.PublishCommand {
	return &npcdeck.PublishCommand{Rules: rules, PublishedBy: f.operatorID}
}

func (f fixture) retireCommand() *npcdeck.RetireCommand {
	return &npcdeck.RetireCommand{RetiredBy: f.operatorID}
}

// 合計 8 枚を満たすルールの値にして、公開までの移り方を小さく確かめる
func smallRules() gamerules.Deck {
	return gamerules.Deck{TotalCardCount: 8, MaxCardDuplicates: 4}
}

func (f fixture) published(t *testing.T) npcdeck.State {
	t.Helper()
	state, _ := apply(t, f.fullDraft(t), f.publishCommand(smallRules()), changedAt)
	return state
}

func (f fixture) retired(t *testing.T) npcdeck.State {
	t.Helper()
	state, _ := apply(t, f.published(t), f.retireCommand(), changedAt)
	return state
}

func TestDecide_RegisterEmptyMakesDraftWithoutSource(t *testing.T) {
	f := newFixture(t)

	events, err := npcdeck.Decide(npcdeck.InitialState(), f.registerCommand(), registeredAt)

	require.NoError(t, err)
	require.Equal(t, []npcdeck.Event{&npcdeck.RegisteredEvent{
		EventMeta:    npcdeck.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:     publicID,
		Name:         f.name,
		RegisteredBy: f.operatorID,
	}}, events)
	require.Equal(t, npcdeck.NPCDeck{
		ID: f.id, Version: 1, PublicID: publicID, Name: f.name, Status: npcdeck.StatusDraft,
		Cards: map[presetdeck.CardRef]presetdeck.CardCount{},
	}, f.register(t))
}

func TestDecide_RegisterCopyKeepsSourceAndCardsInOrder(t *testing.T) {
	f := newFixture(t)
	source := npcdecktest.Source(t, presetID, 3)
	cmd := f.registerCommand()
	cmd.Source = &source
	cmd.Cards = map[presetdeck.CardRef]presetdeck.CardCount{
		f.potion: presetdecktest.CardCount(t, 3),
		f.dragon: presetdecktest.CardCount(t, 2),
	}

	state, events := apply(t, npcdeck.InitialState(), cmd, registeredAt)

	require.Equal(t, []npcdeck.CardEntry{
		{Card: f.dragon, Count: presetdecktest.CardCount(t, 2)},
		{Card: f.potion, Count: presetdecktest.CardCount(t, 3)},
	}, events[0].(*npcdeck.RegisteredEvent).Cards)
	deck := state.(npcdeck.NPCDeck)
	require.Equal(t, &source, events[0].(*npcdeck.RegisteredEvent).Source)
	require.Equal(t, 5, deck.TotalCount())
	require.Equal(t, npcdeck.StatusDraft, deck.Status)
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)

	_, err := npcdeck.Decide(f.register(t), f.registerCommand(), registeredAt)

	require.ErrorIs(t, err, npcdeck.ErrInvalidState)
}

func TestDecide_RenameChangesNameWithNextVersion(t *testing.T) {
	f := newFixture(t)
	newName := presetdecktest.Name(t, "水のデッキ")

	state, events := apply(t, f.register(t), &npcdeck.RenameCommand{Name: newName, RenamedBy: f.operatorID}, changedAt)

	require.Equal(t, []npcdeck.Event{&npcdeck.RenamedEvent{
		EventMeta: npcdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Name:      newName,
		RenamedBy: f.operatorID,
	}}, events)
	deck := state.(npcdeck.NPCDeck)
	require.Equal(t, newName, deck.Name)
	require.Equal(t, int64(2), deck.Version)
}

// 版を進めないため（画面で見た版を古くしないため）
func TestDecide_EmitsNothingWhenNothingChanges(t *testing.T) {
	f := newFixture(t)
	withDragon := f.setCounts(t, f.register(t), map[presetdeck.CardRef]int{f.dragon: 2})
	tests := []struct {
		name  string
		state npcdeck.State
		cmd   npcdeck.Command
	}{
		{name: "same_name", state: withDragon, cmd: &npcdeck.RenameCommand{Name: f.name, RenamedBy: f.operatorID}},
		{name: "same_count", state: withDragon, cmd: f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 2})},
		{name: "zero_count_of_card_not_in_deck", state: withDragon, cmd: f.setCountsCommand(t, map[presetdeck.CardRef]int{f.potion: 0})},
		{name: "no_cards", state: withDragon, cmd: f.setCountsCommand(t, map[presetdeck.CardRef]int{})},
		{name: "publish_published", state: f.published(t), cmd: f.publishCommand(smallRules())},
		{name: "retire_retired", state: f.retired(t), cmd: f.retireCommand()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := npcdeck.Decide(tt.state, tt.cmd, changedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

func TestDecide_SetCardCountsEmitsDeltasOfChangedCards(t *testing.T) {
	f := newFixture(t)
	slime := presetdecktest.CardRef(t, creature.Types().EntityType, 22)
	state := f.setCounts(t, f.register(t), map[presetdeck.CardRef]int{f.dragon: 3, slime: 1})

	_, events := apply(t, state, f.setCountsCommand(t, map[presetdeck.CardRef]int{f.potion: 4, f.dragon: 1, slime: 1}), changedAt)

	require.Equal(t, []npcdeck.Event{&npcdeck.CardCountsChangedEvent{
		EventMeta: npcdeck.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Changes: []npcdeck.CardCountChange{
			{Card: f.dragon, Delta: -2},
			{Card: f.potion, Delta: 4},
		},
		ChangedBy: f.operatorID,
	}}, events)
}

func TestDecide_RejectsTotalCountOverRule(t *testing.T) {
	f := newFixture(t)
	cmd := f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 3, f.potion: 2})
	cmd.Rules.TotalCardCount = 5
	state, _ := apply(t, f.register(t), cmd, changedAt)
	more := f.setCountsCommand(t, map[presetdeck.CardRef]int{f.potion: 3})
	more.Rules.TotalCardCount = 5

	_, err := npcdeck.Decide(state, more, changedAt)

	require.ErrorIs(t, err, npcdeck.ErrTotalCountTooLarge)
}

func TestDecide_RejectsCardCountOverRule(t *testing.T) {
	f := newFixture(t)
	atMax := f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 2})
	atMax.Rules.MaxCardDuplicates = 2
	over := f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 3})
	over.Rules.MaxCardDuplicates = 2

	_, err := npcdeck.Decide(f.register(t), atMax, changedAt)
	require.NoError(t, err)
	_, err = npcdeck.Decide(f.register(t), over, changedAt)

	require.ErrorIs(t, err, npcdeck.ErrCardCountOutOfRange)
}

func TestEvolve_CardCountsChangedEventSetsAndRemovesCards(t *testing.T) {
	f := newFixture(t)
	state := f.setCounts(t, f.register(t), map[presetdeck.CardRef]int{f.dragon: 3, f.potion: 2})
	state = f.setCounts(t, state, map[presetdeck.CardRef]int{f.dragon: 0, f.potion: 4})

	deck := state.(npcdeck.NPCDeck)

	require.Equal(t, map[presetdeck.CardRef]presetdeck.CardCount{f.potion: presetdecktest.CardCount(t, 4)}, deck.Cards)
	require.Equal(t, 4, deck.TotalCount())
	require.Equal(t, int64(3), deck.Version)
}

func TestEvolve_RejectsDeltaOutOfRange(t *testing.T) {
	f := newFixture(t)
	event := &npcdeck.CardCountsChangedEvent{
		EventMeta: npcdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Changes:   []npcdeck.CardCountChange{{Card: f.dragon, Delta: -1}},
		ChangedBy: f.operatorID,
	}

	_, err := npcdeck.Evolve(f.register(t), event)

	require.ErrorIs(t, err, presetdeck.ErrCardCountOutOfRange)
}

func TestEvolve_DoesNotChangeTheGivenState(t *testing.T) {
	f := newFixture(t)
	before := f.setCounts(t, f.register(t), map[presetdeck.CardRef]int{f.dragon: 3})

	f.setCounts(t, before, map[presetdeck.CardRef]int{f.dragon: 1})

	require.Equal(t, presetdecktest.CardCount(t, 3), before.(npcdeck.NPCDeck).Cards[f.dragon])
}

func TestDecide_PublishMakesPublishedWhenTotalMatchesRule(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.fullDraft(t), f.publishCommand(smallRules()), changedAt)

	require.Equal(t, []npcdeck.Event{&npcdeck.PublishedEvent{
		EventMeta:   npcdeck.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		PublishedBy: f.operatorID,
	}}, events)
	require.Equal(t, npcdeck.StatusPublished, state.(npcdeck.NPCDeck).Status)
}

// 合計は、コマンドで渡されたルールの値とちょうど同じでなければならない（多くても少なくても公開できない）
func TestDecide_RejectsPublishUnlessTotalMatchesRule(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name  string
		total int
	}{
		{name: "fewer", total: 9},
		{name: "more", total: 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := npcdeck.Decide(f.fullDraft(t), f.publishCommand(gamerules.Deck{TotalCardCount: tt.total, MaxCardDuplicates: 4}), changedAt)

			require.ErrorIs(t, err, npcdeck.ErrTotalMismatch)
		})
	}
}

func TestDecide_RetiredCanBePublishedAgainButOnlyWhenRuleIsMet(t *testing.T) {
	f := newFixture(t)

	_, err := npcdeck.Decide(f.retired(t), f.publishCommand(gamerules.Deck{TotalCardCount: 40, MaxCardDuplicates: 4}), changedAt)
	require.ErrorIs(t, err, npcdeck.ErrTotalMismatch)

	state, _ := apply(t, f.retired(t), f.publishCommand(smallRules()), changedAt)
	require.Equal(t, npcdeck.StatusPublished, state.(npcdeck.NPCDeck).Status)
}

func TestDecide_RetireMakesRetiredOnlyFromPublished(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.published(t), f.retireCommand(), changedAt)

	require.Equal(t, []npcdeck.Event{&npcdeck.RetiredEvent{
		EventMeta: npcdeck.EventMeta{EntityID: f.id, AggregateVersion: 4, OccurredAt: changedAt},
		RetiredBy: f.operatorID,
	}}, events)
	require.Equal(t, npcdeck.StatusRetired, state.(npcdeck.NPCDeck).Status)

	_, err := npcdeck.Decide(f.register(t), f.retireCommand(), changedAt)
	require.ErrorIs(t, err, npcdeck.ErrInvalidStatusTransition)
}

// 公開した中身が途中で変わらないよう、準備中以外では名前・枚数を変えられず、消せない。準備中に戻す操作も無い
func TestDecide_RejectsEditAndDeleteUnlessDraft(t *testing.T) {
	f := newFixture(t)
	commands := []struct {
		name string
		cmd  npcdeck.Command
	}{
		{name: "rename", cmd: &npcdeck.RenameCommand{Name: presetdecktest.Name(t, "水のデッキ"), RenamedBy: f.operatorID}},
		{name: "set_card_counts", cmd: f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 1})},
		{name: "delete", cmd: &npcdeck.DeleteCommand{DeletedBy: f.operatorID}},
	}
	states := []struct {
		name  string
		state npcdeck.State
	}{
		{name: "published", state: f.published(t)},
		{name: "retired", state: f.retired(t)},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := npcdeck.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, npcdeck.ErrNotDraft)
			})
		}
	}
}

func TestDecide_DeleteMakesDeletedNPCDeck(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.register(t), &npcdeck.DeleteCommand{DeletedBy: f.operatorID}, changedAt)

	require.Equal(t, []npcdeck.Event{&npcdeck.DeletedEvent{
		EventMeta: npcdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		DeletedBy: f.operatorID,
	}}, events)
	require.Equal(t, npcdeck.DeletedNPCDeck{ID: f.id, Version: 2}, state)
}

func TestDecide_RejectsChangesUnlessRegistered(t *testing.T) {
	f := newFixture(t)
	deleted, _ := apply(t, f.register(t), &npcdeck.DeleteCommand{DeletedBy: f.operatorID}, changedAt)
	commands := []struct {
		name string
		cmd  npcdeck.Command
	}{
		{name: "rename", cmd: &npcdeck.RenameCommand{Name: presetdecktest.Name(t, "水のデッキ"), RenamedBy: f.operatorID}},
		{name: "set_card_counts", cmd: f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 1})},
		{name: "publish", cmd: f.publishCommand(smallRules())},
		{name: "retire", cmd: f.retireCommand()},
		{name: "delete", cmd: &npcdeck.DeleteCommand{DeletedBy: f.operatorID}},
	}
	states := []struct {
		name  string
		state npcdeck.State
	}{
		{name: "unregistered", state: npcdeck.InitialState()},
		{name: "deleted", state: deleted},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := npcdeck.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, npcdeck.ErrInvalidState)
			})
		}
	}
}

func TestStatus_TransitionsFollowRules(t *testing.T) {
	tests := []struct {
		name        string
		from        npcdeck.Status
		publishTo   npcdeck.Status
		publishDiff bool
		retireTo    npcdeck.Status
		retireDiff  bool
		retireErr   error
	}{
		{name: "draft", from: npcdeck.StatusDraft, publishTo: npcdeck.StatusPublished, publishDiff: true, retireTo: npcdeck.StatusDraft, retireErr: npcdeck.ErrInvalidStatusTransition},
		{name: "published", from: npcdeck.StatusPublished, publishTo: npcdeck.StatusPublished, retireTo: npcdeck.StatusRetired, retireDiff: true},
		{name: "retired", from: npcdeck.StatusRetired, publishTo: npcdeck.StatusPublished, publishDiff: true, retireTo: npcdeck.StatusRetired},
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
	_, err := npcdeck.NewStatus("archived")

	require.ErrorIs(t, err, npcdeck.ErrStatusUnknown)
}

func TestNewSource_RejectsMissingIDOrRevision(t *testing.T) {
	_, err := npcdeck.NewSource(uuid.Nil, 1)
	require.Error(t, err)
	_, err = npcdeck.NewSource(presetID, 0)
	require.Error(t, err)
}

// 公開中の名前だけでなく、状態によらず、すべての NPC のデッキで比べる。比べ方はカード名と同じ
func TestNameKey_TreatsWidthAndCaseAsSame(t *testing.T) {
	require.Equal(t,
		npcdeck.NameKey(presetdecktest.Name(t, "Ｆire デッキ")),
		npcdeck.NameKey(presetdecktest.Name(t, "fire デッキ")))
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	f := newFixture(t)
	meta := npcdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt}
	source := npcdecktest.Source(t, presetID, 3)
	events := []npcdeck.Event{
		&npcdeck.RegisteredEvent{
			EventMeta: meta, PublicID: publicID, Name: f.name, Source: &source,
			Cards: []npcdeck.CardEntry{
				{Card: f.dragon, Count: presetdecktest.CardCount(t, 2)},
				{Card: f.potion, Count: presetdecktest.CardCount(t, 3)},
			},
			RegisteredBy: f.operatorID,
		},
		&npcdeck.RenamedEvent{EventMeta: meta, Name: presetdecktest.Name(t, "水のデッキ"), RenamedBy: f.operatorID},
		&npcdeck.CardCountsChangedEvent{EventMeta: meta, Changes: []npcdeck.CardCountChange{
			{Card: f.dragon, Delta: -2},
			{Card: f.potion, Delta: 3},
		}, ChangedBy: f.operatorID},
		&npcdeck.PublishedEvent{EventMeta: meta, PublishedBy: f.operatorID},
		&npcdeck.RetiredEvent{EventMeta: meta, RetiredBy: f.operatorID},
		&npcdeck.DeletedEvent{EventMeta: meta, DeletedBy: f.operatorID},
	}
	for _, event := range events {
		t.Run(event.EventType().String(), func(t *testing.T) {
			require.Equal(t, 1, event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := npcdeck.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, event, restored)
		})
	}
}

// 写していないときは、source・cards を payload に出さない
func TestEvent_RegisteredWithoutSourceOmitsSourceAndCards(t *testing.T) {
	f := newFixture(t)
	event := &npcdeck.RegisteredEvent{
		EventMeta: npcdeck.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:  publicID, Name: f.name, RegisteredBy: f.operatorID,
	}

	payload, err := json.Marshal(event)
	require.NoError(t, err)

	require.JSONEq(t, `{"public_id":"11111111-2222-3333-4444-555555555555","name":"炎のデッキ","registered_by":10}`, string(payload))
	restored, err := npcdeck.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), 1, payload)
	require.NoError(t, err)
	require.Equal(t, event, restored)
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := npcdeck.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(npcdeck.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}
