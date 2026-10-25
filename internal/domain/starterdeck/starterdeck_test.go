package starterdeck_test

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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck/presetdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck/starterdecktest"
)

var (
	registeredAt = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	changedAt    = time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
	presetID     = uuid.MustParse("66666666-7777-8888-9999-000000000000")
)

type fixture struct {
	id       starterdeck.ID
	name     presetdeck.Name
	operator operator.ID
	dragon   presetdeck.CardRef
	potion   presetdeck.CardRef
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{
		id:       starterdecktest.ID(t, 30),
		name:     presetdecktest.Name(t, "炎のデッキ"),
		operator: operatortest.ID(t, 10),
		dragon:   presetdecktest.CardRef(t, creature.Types().EntityType, 20),
		potion:   presetdecktest.CardRef(t, item.Types().EntityType, 21),
	}
}

func (f fixture) registerCommand() *starterdeck.RegisterCommand {
	return &starterdeck.RegisterCommand{ID: f.id, PublicID: publicID, Name: f.name, RegisteredBy: f.operator}
}

func apply(t *testing.T, state starterdeck.State, cmd starterdeck.Command, now time.Time) (starterdeck.State, []starterdeck.Event) {
	t.Helper()
	events, err := starterdeck.Decide(state, cmd, now)
	require.NoError(t, err)
	for _, event := range events {
		state, err = starterdeck.Evolve(state, event)
		require.NoError(t, err)
	}
	return state, events
}

func (f fixture) register(t *testing.T) starterdeck.State {
	t.Helper()
	state, _ := apply(t, starterdeck.InitialState(), f.registerCommand(), registeredAt)
	return state
}

func (f fixture) setCountsCommand(t *testing.T, counts map[presetdeck.CardRef]int) *starterdeck.SetCardCountsCommand {
	t.Helper()
	cardCounts := make(map[presetdeck.CardRef]presetdeck.CardCount, len(counts))
	for card, count := range counts {
		cardCounts[card] = presetdecktest.CardCount(t, count)
	}
	return &starterdeck.SetCardCountsCommand{Counts: cardCounts, Rules: gamerules.Defaults().Deck, ChangedBy: f.operator}
}

func (f fixture) setCounts(t *testing.T, state starterdeck.State, counts map[presetdeck.CardRef]int) starterdeck.State {
	t.Helper()
	state, _ = apply(t, state, f.setCountsCommand(t, counts), changedAt)
	return state
}

// 公開できる、合計がちょうど 40 枚の準備中
func (f fixture) fullDraft(t *testing.T) starterdeck.State {
	t.Helper()
	return f.setCounts(t, f.register(t), map[presetdeck.CardRef]int{f.dragon: 4, f.potion: 4})
}

func (f fixture) publishCommand(rules gamerules.Deck) *starterdeck.PublishCommand {
	return &starterdeck.PublishCommand{Rules: rules, PublishedBy: f.operator}
}

func (f fixture) retireCommand() *starterdeck.RetireCommand {
	return &starterdeck.RetireCommand{RetiredBy: f.operator}
}

// 合計 8 枚を満たすルールの値にして、公開までの移り方を小さく確かめる
func smallRules() gamerules.Deck {
	return gamerules.Deck{TotalCardCount: 8, MaxCardDuplicates: 4}
}

func (f fixture) published(t *testing.T) starterdeck.State {
	t.Helper()
	state, _ := apply(t, f.fullDraft(t), f.publishCommand(smallRules()), changedAt)
	return state
}

func (f fixture) retired(t *testing.T) starterdeck.State {
	t.Helper()
	state, _ := apply(t, f.published(t), f.retireCommand(), changedAt)
	return state
}

func TestDecide_RegisterEmptyMakesDraftWithoutSource(t *testing.T) {
	f := newFixture(t)

	events, err := starterdeck.Decide(starterdeck.InitialState(), f.registerCommand(), registeredAt)

	require.NoError(t, err)
	require.Equal(t, []starterdeck.Event{&starterdeck.RegisteredEvent{
		EventMeta:    starterdeck.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:     publicID,
		Name:         f.name,
		RegisteredBy: f.operator,
	}}, events)
	require.Equal(t, starterdeck.StarterDeck{
		ID: f.id, Version: 1, PublicID: publicID, Name: f.name, Status: starterdeck.StatusDraft,
		Cards: map[presetdeck.CardRef]presetdeck.CardCount{}, RegisteredBy: f.operator, RegisteredAt: registeredAt, UpdatedAt: registeredAt,
	}, f.register(t))
}

func TestDecide_RegisterCopyKeepsSourceAndCardsInOrder(t *testing.T) {
	f := newFixture(t)
	source := starterdecktest.Source(t, presetID, 3)
	cmd := f.registerCommand()
	cmd.Source = &source
	cmd.Cards = map[presetdeck.CardRef]presetdeck.CardCount{
		f.potion: presetdecktest.CardCount(t, 3),
		f.dragon: presetdecktest.CardCount(t, 2),
	}

	state, events := apply(t, starterdeck.InitialState(), cmd, registeredAt)

	require.Equal(t, []starterdeck.CardEntry{
		{Card: f.dragon, Count: presetdecktest.CardCount(t, 2)},
		{Card: f.potion, Count: presetdecktest.CardCount(t, 3)},
	}, events[0].(*starterdeck.RegisteredEvent).Cards)
	deck := state.(starterdeck.StarterDeck)
	require.Equal(t, &source, deck.Source)
	require.Equal(t, 5, deck.TotalCount())
	require.Equal(t, starterdeck.StatusDraft, deck.Status)
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)

	_, err := starterdeck.Decide(f.register(t), f.registerCommand(), registeredAt)

	require.ErrorIs(t, err, starterdeck.ErrInvalidState)
}

func TestDecide_RenameChangesNameWithNextVersion(t *testing.T) {
	f := newFixture(t)
	newName := presetdecktest.Name(t, "水のデッキ")

	state, events := apply(t, f.register(t), &starterdeck.RenameCommand{Name: newName, RenamedBy: f.operator}, changedAt)

	require.Equal(t, []starterdeck.Event{&starterdeck.RenamedEvent{
		EventMeta: starterdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Name:      newName,
		RenamedBy: f.operator,
	}}, events)
	deck := state.(starterdeck.StarterDeck)
	require.Equal(t, newName, deck.Name)
	require.Equal(t, int64(2), deck.Version)
	require.Equal(t, changedAt, deck.UpdatedAt)
}

// 版を進めないため（画面で見た版を古くしないため）
func TestDecide_EmitsNothingWhenNothingChanges(t *testing.T) {
	f := newFixture(t)
	withDragon := f.setCounts(t, f.register(t), map[presetdeck.CardRef]int{f.dragon: 2})
	tests := []struct {
		name  string
		state starterdeck.State
		cmd   starterdeck.Command
	}{
		{name: "same_name", state: withDragon, cmd: &starterdeck.RenameCommand{Name: f.name, RenamedBy: f.operator}},
		{name: "same_count", state: withDragon, cmd: f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 2})},
		{name: "zero_count_of_card_not_in_deck", state: withDragon, cmd: f.setCountsCommand(t, map[presetdeck.CardRef]int{f.potion: 0})},
		{name: "no_cards", state: withDragon, cmd: f.setCountsCommand(t, map[presetdeck.CardRef]int{})},
		{name: "publish_published", state: f.published(t), cmd: f.publishCommand(smallRules())},
		{name: "retire_retired", state: f.retired(t), cmd: f.retireCommand()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := starterdeck.Decide(tt.state, tt.cmd, changedAt)

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

	require.Equal(t, []starterdeck.Event{&starterdeck.CardCountsChangedEvent{
		EventMeta: starterdeck.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Changes: []starterdeck.CardCountChange{
			{Card: f.dragon, Delta: -2},
			{Card: f.potion, Delta: 4},
		},
		ChangedBy: f.operator,
	}}, events)
}

func TestDecide_RejectsTotalCountOverRule(t *testing.T) {
	f := newFixture(t)
	cmd := f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 3, f.potion: 2})
	cmd.Rules.TotalCardCount = 5
	state, _ := apply(t, f.register(t), cmd, changedAt)
	more := f.setCountsCommand(t, map[presetdeck.CardRef]int{f.potion: 3})
	more.Rules.TotalCardCount = 5

	_, err := starterdeck.Decide(state, more, changedAt)

	require.ErrorIs(t, err, starterdeck.ErrTotalCountTooLarge)
}

func TestDecide_RejectsCardCountOverRule(t *testing.T) {
	f := newFixture(t)
	atMax := f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 2})
	atMax.Rules.MaxCardDuplicates = 2
	over := f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 3})
	over.Rules.MaxCardDuplicates = 2

	_, err := starterdeck.Decide(f.register(t), atMax, changedAt)
	require.NoError(t, err)
	_, err = starterdeck.Decide(f.register(t), over, changedAt)

	require.ErrorIs(t, err, starterdeck.ErrCardCountOutOfRange)
}

func TestEvolve_CardCountsChangedEventSetsAndRemovesCards(t *testing.T) {
	f := newFixture(t)
	state := f.setCounts(t, f.register(t), map[presetdeck.CardRef]int{f.dragon: 3, f.potion: 2})
	state = f.setCounts(t, state, map[presetdeck.CardRef]int{f.dragon: 0, f.potion: 4})

	deck := state.(starterdeck.StarterDeck)

	require.Equal(t, map[presetdeck.CardRef]presetdeck.CardCount{f.potion: presetdecktest.CardCount(t, 4)}, deck.Cards)
	require.Equal(t, 4, deck.TotalCount())
	require.Equal(t, int64(3), deck.Version)
}

func TestEvolve_RejectsDeltaOutOfRange(t *testing.T) {
	f := newFixture(t)
	event := &starterdeck.CardCountsChangedEvent{
		EventMeta: starterdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Changes:   []starterdeck.CardCountChange{{Card: f.dragon, Delta: -1}},
		ChangedBy: f.operator,
	}

	_, err := starterdeck.Evolve(f.register(t), event)

	require.ErrorIs(t, err, presetdeck.ErrCardCountOutOfRange)
}

func TestEvolve_DoesNotChangeTheGivenState(t *testing.T) {
	f := newFixture(t)
	before := f.setCounts(t, f.register(t), map[presetdeck.CardRef]int{f.dragon: 3})

	f.setCounts(t, before, map[presetdeck.CardRef]int{f.dragon: 1})

	require.Equal(t, presetdecktest.CardCount(t, 3), before.(starterdeck.StarterDeck).Cards[f.dragon])
}

func TestDecide_PublishMakesPublishedWhenTotalMatchesRule(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.fullDraft(t), f.publishCommand(smallRules()), changedAt)

	require.Equal(t, []starterdeck.Event{&starterdeck.PublishedEvent{
		EventMeta:   starterdeck.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		PublishedBy: f.operator,
	}}, events)
	require.Equal(t, starterdeck.StatusPublished, state.(starterdeck.StarterDeck).Status)
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
			_, err := starterdeck.Decide(f.fullDraft(t), f.publishCommand(gamerules.Deck{TotalCardCount: tt.total, MaxCardDuplicates: 4}), changedAt)

			require.ErrorIs(t, err, starterdeck.ErrTotalMismatch)
		})
	}
}

func TestDecide_RetiredCanBePublishedAgainButOnlyWhenRuleIsMet(t *testing.T) {
	f := newFixture(t)

	_, err := starterdeck.Decide(f.retired(t), f.publishCommand(gamerules.Deck{TotalCardCount: 40, MaxCardDuplicates: 4}), changedAt)
	require.ErrorIs(t, err, starterdeck.ErrTotalMismatch)

	state, _ := apply(t, f.retired(t), f.publishCommand(smallRules()), changedAt)
	require.Equal(t, starterdeck.StatusPublished, state.(starterdeck.StarterDeck).Status)
}

func TestDecide_RetireMakesRetiredOnlyFromPublished(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.published(t), f.retireCommand(), changedAt)

	require.Equal(t, []starterdeck.Event{&starterdeck.RetiredEvent{
		EventMeta: starterdeck.EventMeta{EntityID: f.id, AggregateVersion: 4, OccurredAt: changedAt},
		RetiredBy: f.operator,
	}}, events)
	require.Equal(t, starterdeck.StatusRetired, state.(starterdeck.StarterDeck).Status)

	_, err := starterdeck.Decide(f.register(t), f.retireCommand(), changedAt)
	require.ErrorIs(t, err, starterdeck.ErrInvalidStatusTransition)
}

// 公開した中身が途中で変わらないよう、準備中以外では名前・枚数を変えられず、消せない。準備中に戻す操作も無い
func TestDecide_RejectsEditAndDeleteUnlessDraft(t *testing.T) {
	f := newFixture(t)
	commands := []struct {
		name string
		cmd  starterdeck.Command
	}{
		{name: "rename", cmd: &starterdeck.RenameCommand{Name: presetdecktest.Name(t, "水のデッキ"), RenamedBy: f.operator}},
		{name: "set_card_counts", cmd: f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 1})},
		{name: "delete", cmd: &starterdeck.DeleteCommand{DeletedBy: f.operator}},
	}
	states := []struct {
		name  string
		state starterdeck.State
	}{
		{name: "published", state: f.published(t)},
		{name: "retired", state: f.retired(t)},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := starterdeck.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, starterdeck.ErrNotDraft)
			})
		}
	}
}

func TestDecide_DeleteMakesDeletedStarterDeck(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.register(t), &starterdeck.DeleteCommand{DeletedBy: f.operator}, changedAt)

	require.Equal(t, []starterdeck.Event{&starterdeck.DeletedEvent{
		EventMeta: starterdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		DeletedBy: f.operator,
	}}, events)
	require.Equal(t, starterdeck.DeletedStarterDeck{ID: f.id, Version: 2}, state)
}

func TestDecide_RejectsChangesUnlessRegistered(t *testing.T) {
	f := newFixture(t)
	deleted, _ := apply(t, f.register(t), &starterdeck.DeleteCommand{DeletedBy: f.operator}, changedAt)
	commands := []struct {
		name string
		cmd  starterdeck.Command
	}{
		{name: "rename", cmd: &starterdeck.RenameCommand{Name: presetdecktest.Name(t, "水のデッキ"), RenamedBy: f.operator}},
		{name: "set_card_counts", cmd: f.setCountsCommand(t, map[presetdeck.CardRef]int{f.dragon: 1})},
		{name: "publish", cmd: f.publishCommand(smallRules())},
		{name: "retire", cmd: f.retireCommand()},
		{name: "delete", cmd: &starterdeck.DeleteCommand{DeletedBy: f.operator}},
	}
	states := []struct {
		name  string
		state starterdeck.State
	}{
		{name: "unregistered", state: starterdeck.InitialState()},
		{name: "deleted", state: deleted},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := starterdeck.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, starterdeck.ErrInvalidState)
			})
		}
	}
}

func TestStatus_TransitionsFollowRules(t *testing.T) {
	tests := []struct {
		name        string
		from        starterdeck.Status
		publishTo   starterdeck.Status
		publishDiff bool
		retireTo    starterdeck.Status
		retireDiff  bool
		retireErr   error
	}{
		{name: "draft", from: starterdeck.StatusDraft, publishTo: starterdeck.StatusPublished, publishDiff: true, retireTo: starterdeck.StatusDraft, retireErr: starterdeck.ErrInvalidStatusTransition},
		{name: "published", from: starterdeck.StatusPublished, publishTo: starterdeck.StatusPublished, retireTo: starterdeck.StatusRetired, retireDiff: true},
		{name: "retired", from: starterdeck.StatusRetired, publishTo: starterdeck.StatusPublished, publishDiff: true, retireTo: starterdeck.StatusRetired},
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
	_, err := starterdeck.NewStatus("archived")

	require.ErrorIs(t, err, starterdeck.ErrStatusUnknown)
}

func TestNewSource_RejectsMissingIDOrRevision(t *testing.T) {
	_, err := starterdeck.NewSource(uuid.Nil, 1)
	require.Error(t, err)
	_, err = starterdeck.NewSource(presetID, 0)
	require.Error(t, err)
}

// 公開中の名前だけでなく、状態によらず、すべてのスターターデッキで比べる。比べ方はカード名と同じ
func TestNameKey_TreatsWidthAndCaseAsSame(t *testing.T) {
	require.Equal(t,
		starterdeck.NameKey(presetdecktest.Name(t, "Ｆire デッキ")),
		starterdeck.NameKey(presetdecktest.Name(t, "fire デッキ")))
}

func TestPublishedQuota_CountsOnceForTheGame(t *testing.T) {
	rules, err := gamerules.NewID(5)
	require.NoError(t, err)

	quota := starterdeck.PublishedQuota(rules)

	require.Equal(t, starterdeck.PublishedQuotaType, quota.Type())
	require.Equal(t, int64(5), quota.OwnerID())
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	f := newFixture(t)
	meta := starterdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt}
	source := starterdecktest.Source(t, presetID, 3)
	events := []starterdeck.Event{
		&starterdeck.RegisteredEvent{
			EventMeta: meta, PublicID: publicID, Name: f.name, Source: &source,
			Cards: []starterdeck.CardEntry{
				{Card: f.dragon, Count: presetdecktest.CardCount(t, 2)},
				{Card: f.potion, Count: presetdecktest.CardCount(t, 3)},
			},
			RegisteredBy: f.operator,
		},
		&starterdeck.RenamedEvent{EventMeta: meta, Name: presetdecktest.Name(t, "水のデッキ"), RenamedBy: f.operator},
		&starterdeck.CardCountsChangedEvent{EventMeta: meta, Changes: []starterdeck.CardCountChange{
			{Card: f.dragon, Delta: -2},
			{Card: f.potion, Delta: 3},
		}, ChangedBy: f.operator},
		&starterdeck.PublishedEvent{EventMeta: meta, PublishedBy: f.operator},
		&starterdeck.RetiredEvent{EventMeta: meta, RetiredBy: f.operator},
		&starterdeck.DeletedEvent{EventMeta: meta, DeletedBy: f.operator},
	}
	for _, event := range events {
		t.Run(event.EventType().String(), func(t *testing.T) {
			require.Equal(t, 1, event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := starterdeck.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, event, restored)
		})
	}
}

// 写していないときは、source・cards を payload に出さない
func TestEvent_RegisteredWithoutSourceOmitsSourceAndCards(t *testing.T) {
	f := newFixture(t)
	event := &starterdeck.RegisteredEvent{
		EventMeta: starterdeck.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:  publicID, Name: f.name, RegisteredBy: f.operator,
	}

	payload, err := json.Marshal(event)
	require.NoError(t, err)

	require.JSONEq(t, `{"public_id":"11111111-2222-3333-4444-555555555555","name":"炎のデッキ","registered_by":10}`, string(payload))
	restored, err := starterdeck.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), 1, payload)
	require.NoError(t, err)
	require.Equal(t, event, restored)
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := starterdeck.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(starterdeck.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}
