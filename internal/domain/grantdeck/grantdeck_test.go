package grantdeck_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck/grantdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck/presetdecktest"
)

var (
	registeredAt = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	changedAt    = time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
)

type fixture struct {
	id         grantdeck.ID
	name       presetdeck.Name
	operatorID operator.ID
	dragon     card.Ref
	potion     card.Ref
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{
		id:         grantdecktest.ID(t, 30),
		name:       presetdecktest.Name(t, "炎のデッキ"),
		operatorID: operatortest.ID(t, 10),
		dragon:     cardtest.Ref(t, creature.Types().EntityType, 20),
		potion:     cardtest.Ref(t, item.Types().EntityType, 21),
	}
}

func (f fixture) registerCommand() *grantdeck.RegisterCommand {
	return &grantdeck.RegisterCommand{ID: f.id, PublicID: publicID, Name: f.name, RegisteredBy: f.operatorID}
}

func apply(t *testing.T, state grantdeck.State, cmd grantdeck.Command, now time.Time) (grantdeck.State, []grantdeck.Event) {
	t.Helper()
	events, err := grantdeck.Decide(state, cmd, now)
	require.NoError(t, err)
	for _, event := range events {
		state, err = grantdeck.Evolve(state, event)
		require.NoError(t, err)
	}
	return state, events
}

func (f fixture) register(t *testing.T) grantdeck.State {
	t.Helper()
	state, _ := apply(t, grantdeck.InitialState(), f.registerCommand(), registeredAt)
	return state
}

func (f fixture) setCountsCommand(t *testing.T, counts map[card.Ref]int) *grantdeck.SetCardCountsCommand {
	t.Helper()
	set := make(map[card.Ref]card.DeckQuantity, len(counts))
	var remove []card.Ref
	for ref, count := range counts {
		if count == 0 {
			remove = append(remove, ref)
			continue
		}
		set[ref] = cardtest.DeckQuantity(t, count)
	}
	return &grantdeck.SetCardCountsCommand{Set: set, Remove: remove, Rules: gamerules.Defaults().Deck, ChangedBy: f.operatorID}
}

func (f fixture) setCounts(t *testing.T, state grantdeck.State, counts map[card.Ref]int) grantdeck.State {
	t.Helper()
	state, _ = apply(t, state, f.setCountsCommand(t, counts), changedAt)
	return state
}

// 公開できる、合計がちょうど 40 枚の準備中
func (f fixture) fullDraft(t *testing.T) grantdeck.State {
	t.Helper()
	return f.setCounts(t, f.register(t), map[card.Ref]int{f.dragon: 4, f.potion: 4})
}

func (f fixture) publishCommand(rules gamerules.Deck) *grantdeck.PublishCommand {
	return &grantdeck.PublishCommand{Rules: rules, PublishedBy: f.operatorID}
}

func (f fixture) retireCommand() *grantdeck.RetireCommand {
	return &grantdeck.RetireCommand{RetiredBy: f.operatorID}
}

// 合計 8 枚を満たすルールの値にして、公開までの移り方を小さく確かめる
func smallRules(t *testing.T) gamerules.Deck {
	return rulesWithTotal(t, 8)
}

func rulesWithTotal(t *testing.T, total int) gamerules.Deck {
	rules := gamerules.Defaults().Deck
	rules.TotalCardCount = cardtest.DeckTotalCount(t, total)
	return rules
}

func (f fixture) published(t *testing.T) grantdeck.State {
	t.Helper()
	state, _ := apply(t, f.fullDraft(t), f.publishCommand(smallRules(t)), changedAt)
	return state
}

func (f fixture) retired(t *testing.T) grantdeck.State {
	t.Helper()
	state, _ := apply(t, f.published(t), f.retireCommand(), changedAt)
	return state
}

func TestDecide_RegisterEmptyMakesDraftWithoutSource(t *testing.T) {
	f := newFixture(t)

	events, err := grantdeck.Decide(grantdeck.InitialState(), f.registerCommand(), registeredAt)

	require.NoError(t, err)
	require.Equal(t, []grantdeck.Event{&grantdeck.RegisteredEvent{
		EventMeta:    grantdeck.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:     publicID,
		Name:         f.name,
		RegisteredBy: f.operatorID,
	}}, events)
	require.Equal(t, grantdeck.GrantDeck{
		ID: f.id, Version: 1, PublicID: publicID, Name: f.name, Status: grantdeck.StatusDraft,
		Cards: map[card.Ref]card.DeckQuantity{},
	}, f.register(t))
}

func TestDecide_RegisterCopyKeepsSourceAndCardsInOrder(t *testing.T) {
	f := newFixture(t)
	source := grantdecktest.Source(t, presetdecktest.ID(t, 40), 3)
	cmd := f.registerCommand()
	cmd.Source = &source
	cmd.Cards = map[card.Ref]card.DeckQuantity{
		f.potion: cardtest.DeckQuantity(t, 3),
		f.dragon: cardtest.DeckQuantity(t, 2),
	}

	state, events := apply(t, grantdeck.InitialState(), cmd, registeredAt)

	require.Equal(t, []grantdeck.DeckCard{
		{Card: f.dragon, Quantity: cardtest.DeckQuantity(t, 2)},
		{Card: f.potion, Quantity: cardtest.DeckQuantity(t, 3)},
	}, events[0].(*grantdeck.RegisteredEvent).Cards)
	deck := state.(grantdeck.GrantDeck)
	require.Equal(t, &source, events[0].(*grantdeck.RegisteredEvent).Source)
	require.Equal(t, 5, deck.TotalCount())
	require.Equal(t, grantdeck.StatusDraft, deck.Status)
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	f := newFixture(t)

	_, err := grantdeck.Decide(f.register(t), f.registerCommand(), registeredAt)

	require.ErrorIs(t, err, grantdeck.ErrInvalidState)
}

func TestDecide_RenameChangesNameWithNextVersion(t *testing.T) {
	f := newFixture(t)
	newName := presetdecktest.Name(t, "水のデッキ")

	state, events := apply(t, f.register(t), &grantdeck.RenameCommand{Name: newName, RenamedBy: f.operatorID}, changedAt)

	require.Equal(t, []grantdeck.Event{&grantdeck.RenamedEvent{
		EventMeta: grantdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Name:      newName,
		RenamedBy: f.operatorID,
	}}, events)
	deck := state.(grantdeck.GrantDeck)
	require.Equal(t, newName, deck.Name)
	require.Equal(t, int64(2), deck.Version)
}

// 版を進めないため（画面で見た版を古くしないため）
func TestDecide_EmitsNothingWhenNothingChanges(t *testing.T) {
	f := newFixture(t)
	withDragon := f.setCounts(t, f.register(t), map[card.Ref]int{f.dragon: 2})
	tests := []struct {
		name  string
		state grantdeck.State
		cmd   grantdeck.Command
	}{
		{name: "same_name", state: withDragon, cmd: &grantdeck.RenameCommand{Name: f.name, RenamedBy: f.operatorID}},
		{name: "same_count", state: withDragon, cmd: f.setCountsCommand(t, map[card.Ref]int{f.dragon: 2})},
		{name: "zero_count_of_card_not_in_deck", state: withDragon, cmd: f.setCountsCommand(t, map[card.Ref]int{f.potion: 0})},
		{name: "no_cards", state: withDragon, cmd: f.setCountsCommand(t, map[card.Ref]int{})},
		{name: "publish_published", state: f.published(t), cmd: f.publishCommand(smallRules(t))},
		{name: "retire_retired", state: f.retired(t), cmd: f.retireCommand()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := grantdeck.Decide(tt.state, tt.cmd, changedAt)

			require.NoError(t, err)
			require.Empty(t, events)
		})
	}
}

func TestDecide_SetCardCountsEmitsDeltasOfChangedCards(t *testing.T) {
	f := newFixture(t)
	slime := cardtest.Ref(t, creature.Types().EntityType, 22)
	state := f.setCounts(t, f.register(t), map[card.Ref]int{f.dragon: 3, slime: 1})

	_, events := apply(t, state, f.setCountsCommand(t, map[card.Ref]int{f.potion: 4, f.dragon: 1, slime: 1}), changedAt)

	require.Equal(t, []grantdeck.Event{&grantdeck.CardCountsChangedEvent{
		EventMeta: grantdeck.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		Changes: []grantdeck.CardCountChange{
			{Card: f.dragon, Delta: -2},
			{Card: f.potion, Delta: 4},
		},
		ChangedBy: f.operatorID,
	}}, events)
}

func TestDecide_RejectsTotalCountOverRule(t *testing.T) {
	f := newFixture(t)
	cmd := f.setCountsCommand(t, map[card.Ref]int{f.dragon: 3, f.potion: 2})
	cmd.Rules.TotalCardCount = cardtest.DeckTotalCount(t, 5)
	state, _ := apply(t, f.register(t), cmd, changedAt)
	more := f.setCountsCommand(t, map[card.Ref]int{f.potion: 3})
	more.Rules.TotalCardCount = cardtest.DeckTotalCount(t, 5)

	_, err := grantdeck.Decide(state, more, changedAt)

	require.ErrorIs(t, err, grantdeck.ErrTotalCountTooLarge)
}

func TestEvolve_CardCountsChangedEventSetsAndRemovesCards(t *testing.T) {
	f := newFixture(t)
	state := f.setCounts(t, f.register(t), map[card.Ref]int{f.dragon: 3, f.potion: 2})
	state = f.setCounts(t, state, map[card.Ref]int{f.dragon: 0, f.potion: 4})

	deck := state.(grantdeck.GrantDeck)

	require.Equal(t, map[card.Ref]card.DeckQuantity{f.potion: cardtest.DeckQuantity(t, 4)}, deck.Cards)
	require.Equal(t, 4, deck.TotalCount())
	require.Equal(t, int64(3), deck.Version)
}

func TestEvolve_RejectsDeltaOutOfRange(t *testing.T) {
	f := newFixture(t)
	event := &grantdeck.CardCountsChangedEvent{
		EventMeta: grantdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		Changes:   []grantdeck.CardCountChange{{Card: f.dragon, Delta: -1}},
		ChangedBy: f.operatorID,
	}

	_, err := grantdeck.Evolve(f.register(t), event)

	require.ErrorIs(t, err, card.ErrQuantityNotPositive)
}

func TestEvolve_DoesNotChangeTheGivenState(t *testing.T) {
	f := newFixture(t)
	before := f.setCounts(t, f.register(t), map[card.Ref]int{f.dragon: 3})

	f.setCounts(t, before, map[card.Ref]int{f.dragon: 1})

	require.Equal(t, cardtest.DeckQuantity(t, 3), before.(grantdeck.GrantDeck).Cards[f.dragon])
}

func TestDecide_PublishMakesPublishedWhenTotalMatchesRule(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.fullDraft(t), f.publishCommand(smallRules(t)), changedAt)

	require.Equal(t, []grantdeck.Event{&grantdeck.PublishedEvent{
		EventMeta:   grantdeck.EventMeta{EntityID: f.id, AggregateVersion: 3, OccurredAt: changedAt},
		PublishedBy: f.operatorID,
	}}, events)
	require.Equal(t, grantdeck.StatusPublished, state.(grantdeck.GrantDeck).Status)
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
			_, err := grantdeck.Decide(f.fullDraft(t), f.publishCommand(rulesWithTotal(t, tt.total)), changedAt)

			require.ErrorIs(t, err, grantdeck.ErrTotalMismatch)
		})
	}
}

func TestDecide_RetiredCanBePublishedAgainButOnlyWhenRuleIsMet(t *testing.T) {
	f := newFixture(t)

	_, err := grantdeck.Decide(f.retired(t), f.publishCommand(rulesWithTotal(t, 40)), changedAt)
	require.ErrorIs(t, err, grantdeck.ErrTotalMismatch)

	state, _ := apply(t, f.retired(t), f.publishCommand(smallRules(t)), changedAt)
	require.Equal(t, grantdeck.StatusPublished, state.(grantdeck.GrantDeck).Status)
}

func TestDecide_RetireMakesRetiredOnlyFromPublished(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.published(t), f.retireCommand(), changedAt)

	require.Equal(t, []grantdeck.Event{&grantdeck.RetiredEvent{
		EventMeta: grantdeck.EventMeta{EntityID: f.id, AggregateVersion: 4, OccurredAt: changedAt},
		RetiredBy: f.operatorID,
	}}, events)
	require.Equal(t, grantdeck.StatusRetired, state.(grantdeck.GrantDeck).Status)

	_, err := grantdeck.Decide(f.register(t), f.retireCommand(), changedAt)
	require.ErrorIs(t, err, grantdeck.ErrInvalidStatusTransition)
}

// 公開した中身が途中で変わらないよう、準備中以外では名前・枚数を変えられず、消せない。準備中に戻す操作も無い
func TestDecide_RejectsEditAndDeleteUnlessDraft(t *testing.T) {
	f := newFixture(t)
	commands := []struct {
		name string
		cmd  grantdeck.Command
	}{
		{name: "rename", cmd: &grantdeck.RenameCommand{Name: presetdecktest.Name(t, "水のデッキ"), RenamedBy: f.operatorID}},
		{name: "set_card_counts", cmd: f.setCountsCommand(t, map[card.Ref]int{f.dragon: 1})},
		{name: "delete", cmd: &grantdeck.DeleteCommand{DeletedBy: f.operatorID}},
	}
	states := []struct {
		name  string
		state grantdeck.State
	}{
		{name: "published", state: f.published(t)},
		{name: "retired", state: f.retired(t)},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := grantdeck.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, grantdeck.ErrNotDraft)
			})
		}
	}
}

func TestDecide_DeleteMakesDeletedGrantDeck(t *testing.T) {
	f := newFixture(t)

	state, events := apply(t, f.register(t), &grantdeck.DeleteCommand{DeletedBy: f.operatorID}, changedAt)

	require.Equal(t, []grantdeck.Event{&grantdeck.DeletedEvent{
		EventMeta: grantdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt},
		DeletedBy: f.operatorID,
	}}, events)
	require.Equal(t, grantdeck.DeletedGrantDeck{ID: f.id, Version: 2}, state)
}

func TestDecide_RejectsChangesUnlessRegistered(t *testing.T) {
	f := newFixture(t)
	deleted, _ := apply(t, f.register(t), &grantdeck.DeleteCommand{DeletedBy: f.operatorID}, changedAt)
	commands := []struct {
		name string
		cmd  grantdeck.Command
	}{
		{name: "rename", cmd: &grantdeck.RenameCommand{Name: presetdecktest.Name(t, "水のデッキ"), RenamedBy: f.operatorID}},
		{name: "set_card_counts", cmd: f.setCountsCommand(t, map[card.Ref]int{f.dragon: 1})},
		{name: "publish", cmd: f.publishCommand(smallRules(t))},
		{name: "retire", cmd: f.retireCommand()},
		{name: "delete", cmd: &grantdeck.DeleteCommand{DeletedBy: f.operatorID}},
	}
	states := []struct {
		name  string
		state grantdeck.State
	}{
		{name: "unregistered", state: grantdeck.InitialState()},
		{name: "deleted", state: deleted},
	}
	for _, s := range states {
		for _, c := range commands {
			t.Run(s.name+"_"+c.name, func(t *testing.T) {
				_, err := grantdeck.Decide(s.state, c.cmd, changedAt)

				require.ErrorIs(t, err, grantdeck.ErrInvalidState)
			})
		}
	}
}

func TestStatus_TransitionsFollowRules(t *testing.T) {
	tests := []struct {
		name        string
		from        grantdeck.Status
		publishTo   grantdeck.Status
		publishDiff bool
		retireTo    grantdeck.Status
		retireDiff  bool
		retireErr   error
	}{
		{name: "draft", from: grantdeck.StatusDraft, publishTo: grantdeck.StatusPublished, publishDiff: true, retireTo: grantdeck.StatusDraft, retireErr: grantdeck.ErrInvalidStatusTransition},
		{name: "published", from: grantdeck.StatusPublished, publishTo: grantdeck.StatusPublished, retireTo: grantdeck.StatusRetired, retireDiff: true},
		{name: "retired", from: grantdeck.StatusRetired, publishTo: grantdeck.StatusPublished, publishDiff: true, retireTo: grantdeck.StatusRetired},
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
	_, err := grantdeck.NewStatus("archived")

	require.ErrorIs(t, err, grantdeck.ErrStatusUnknown)
}

// 公開中の名前だけでなく、状態によらず、すべての配布デッキで比べる。比べ方はカード名と同じ
func TestNameKey_TreatsWidthAndCaseAsSame(t *testing.T) {
	require.Equal(t,
		grantdeck.NameKey(presetdecktest.Name(t, "Ｆire デッキ")),
		grantdeck.NameKey(presetdecktest.Name(t, "fire デッキ")))
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	f := newFixture(t)
	meta := grantdeck.EventMeta{EntityID: f.id, AggregateVersion: 2, OccurredAt: changedAt}
	source := grantdecktest.Source(t, presetdecktest.ID(t, 40), 3)
	events := []grantdeck.Event{
		&grantdeck.RegisteredEvent{
			EventMeta: meta, PublicID: publicID, Name: f.name, Source: &source,
			Cards: []grantdeck.DeckCard{
				{Card: f.dragon, Quantity: cardtest.DeckQuantity(t, 2)},
				{Card: f.potion, Quantity: cardtest.DeckQuantity(t, 3)},
			},
			RegisteredBy: f.operatorID,
		},
		&grantdeck.RenamedEvent{EventMeta: meta, Name: presetdecktest.Name(t, "水のデッキ"), RenamedBy: f.operatorID},
		&grantdeck.CardCountsChangedEvent{EventMeta: meta, Changes: []grantdeck.CardCountChange{
			{Card: f.dragon, Delta: -2},
			{Card: f.potion, Delta: 3},
		}, ChangedBy: f.operatorID},
		&grantdeck.PublishedEvent{EventMeta: meta, PublishedBy: f.operatorID},
		&grantdeck.RetiredEvent{EventMeta: meta, RetiredBy: f.operatorID},
		&grantdeck.DeletedEvent{EventMeta: meta, DeletedBy: f.operatorID},
	}
	for _, event := range events {
		t.Run(event.EventType().String(), func(t *testing.T) {
			require.Equal(t, 1, event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := grantdeck.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, event, restored)
		})
	}
}

// 写していないときは、source・cards を payload に出さない
func TestEvent_RegisteredWithoutSourceOmitsSourceAndCards(t *testing.T) {
	f := newFixture(t)
	event := &grantdeck.RegisteredEvent{
		EventMeta: grantdeck.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:  publicID, Name: f.name, RegisteredBy: f.operatorID,
	}

	payload, err := json.Marshal(event)
	require.NoError(t, err)

	require.JSONEq(t, `{"public_id":"11111111-2222-3333-4444-555555555555","name":"炎のデッキ","registered_by":10}`, string(payload))
	restored, err := grantdeck.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), 1, payload)
	require.NoError(t, err)
	require.Equal(t, event, restored)
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := grantdeck.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(grantdeck.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}

func TestDecide_RejectsCardBothSetAndRemoved(t *testing.T) {
	f := newFixture(t)
	cmd := f.setCountsCommand(t, map[card.Ref]int{f.dragon: 2})
	cmd.Remove = []card.Ref{f.dragon}

	_, err := grantdeck.Decide(f.register(t), cmd, changedAt)

	require.ErrorIs(t, err, grantdeck.ErrCardConflict)
}

// 差分を重ねると、持っている枚数より多く引くことになるため、同じカードを 2 回外す指定は 1 回にする
func TestDecide_RemovingSameCardTwiceEmitsOneChange(t *testing.T) {
	f := newFixture(t)
	state := f.setCounts(t, f.register(t), map[card.Ref]int{f.dragon: 3})
	cmd := f.setCountsCommand(t, map[card.Ref]int{})
	cmd.Remove = []card.Ref{f.dragon, f.dragon}

	next, events := apply(t, state, cmd, changedAt)

	require.Len(t, events, 1)
	require.Empty(t, next.(grantdeck.GrantDeck).Cards)
}
