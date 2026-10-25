package starterdeckset_test

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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset/starterdecksettest"
)

var (
	firstSetAt = time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	resetAt    = time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	publicID   = uuid.MustParse("11111111-2222-3333-4444-555555555555")
	rules      = gamerules.Defaults().Deck
)

type fixture struct {
	id       starterdeckset.ID
	operator operator.ID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{id: starterdecksettest.ID(t, 40), operator: operatortest.ID(t, 10)}
}

// 合計がルールの TotalCardCount のプリセットデッキ（MaxCardDuplicates 枚ずつ、cardKinds 種類。最後の 1 種類だけ lastCard にする）
func presetDeck(t *testing.T, publicID uuid.UUID, version int64, name string, lastCard presetdeck.CardRef) presetdeck.PresetDeck {
	t.Helper()
	cardKinds := rules.TotalCardCount / rules.MaxCardDuplicates
	cards := map[presetdeck.CardRef]presetdeck.CardCount{}
	for i := range cardKinds - 1 {
		cards[presetdecktest.CardRef(t, creature.Types().EntityType, int64(100+i))] = presetdecktest.CardCount(t, rules.MaxCardDuplicates)
	}
	cards[lastCard] = presetdecktest.CardCount(t, rules.MaxCardDuplicates)
	return presetdeck.PresetDeck{
		ID:       presetdecktest.ID(t, version*10),
		Version:  version,
		PublicID: publicID,
		Name:     presetdecktest.Name(t, name),
		Cards:    cards,
	}
}

var (
	fireEarthID = uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")
	waterWindID = uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000002")
	lightDarkID = uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000003")
)

func (f fixture) fireEarth(t *testing.T, version int64) presetdeck.PresetDeck {
	t.Helper()
	return presetDeck(t, fireEarthID, version, "火・地属性", presetdecktest.CardRef(t, creature.Types().EntityType, 109))
}

func (f fixture) waterWind(t *testing.T, version int64) presetdeck.PresetDeck {
	t.Helper()
	return presetDeck(t, waterWindID, version, "水・風属性", presetdecktest.CardRef(t, item.Types().EntityType, 200))
}

func (f fixture) lightDark(t *testing.T) presetdeck.PresetDeck {
	t.Helper()
	return presetDeck(t, lightDarkID, 1, "光・闇属性", presetdecktest.CardRef(t, item.Types().EntityType, 201))
}

func (f fixture) setCommand(t *testing.T, decks ...presetdeck.PresetDeck) *starterdeckset.SetCommand {
	t.Helper()
	copied := make([]starterdeckset.Deck, 0, len(decks))
	for _, deck := range decks {
		copied = append(copied, starterdecksettest.Deck(t, deck, rules))
	}
	return &starterdeckset.SetCommand{ID: f.id, PublicID: publicID, Decks: copied, SetBy: f.operator}
}

func (f fixture) set(t *testing.T, state starterdeckset.State, at time.Time, decks ...presetdeck.PresetDeck) (starterdeckset.State, []starterdeckset.Event) {
	t.Helper()
	events, err := starterdeckset.Decide(state, f.setCommand(t, decks...), at)
	require.NoError(t, err)
	for _, event := range events {
		state, err = starterdeckset.Evolve(state, event)
		require.NoError(t, err)
	}
	return state, events
}

func TestNewDeck_CopiesPresetDeckAtThatPoint(t *testing.T) {
	f := newFixture(t)
	deck := f.fireEarth(t, 7)

	copied := starterdecksettest.Deck(t, deck, rules)

	require.Equal(t, fireEarthID, copied.Source().PresetDeckID())
	require.Equal(t, int64(7), copied.Source().Revision())
	require.Equal(t, deck.Name, copied.Name())
	counts := map[presetdeck.CardRef]presetdeck.CardCount{}
	for _, card := range copied.Cards() {
		counts[card.Card()] = card.Count()
	}
	require.Equal(t, deck.Cards, counts)
}

// 選べるのは合計がちょうどルールの TotalCardCount のプリセットデッキだけ
func TestNewDeck_RejectsPresetDeckNotExactlyMaxTotal(t *testing.T) {
	f := newFixture(t)
	deck := f.fireEarth(t, 1)
	for card := range deck.Cards {
		deck.Cards[card] = presetdecktest.CardCount(t, rules.MaxCardDuplicates-1)
		break
	}

	_, err := starterdeckset.NewDeck(deck, rules)

	require.ErrorIs(t, err, starterdeckset.ErrDeckNotSelectable)
}

// 合計はルールの値で決まる（既定の 40 ではない値でも、ちょうどなら選べる）
func TestNewDeck_UsesTotalCardCountOfRule(t *testing.T) {
	f := newFixture(t)
	deck := f.fireEarth(t, 1)
	small := rules
	small.TotalCardCount = deck.TotalCount() - 1

	_, err := starterdeckset.NewDeck(deck, small)
	require.ErrorIs(t, err, starterdeckset.ErrDeckNotSelectable)

	small.TotalCardCount = deck.TotalCount()
	_, err = starterdeckset.NewDeck(deck, small)
	require.NoError(t, err)
}

// 初めての設定で、写しの中身を全部持つイベントを作る（並びは元のプリセットデッキの公開 ID の順）
func TestDecide_FirstSetEmitsDecksSetEventWithCopies(t *testing.T) {
	f := newFixture(t)
	waterWind, fireEarth := f.waterWind(t, 3), f.fireEarth(t, 7)

	state, events := f.set(t, starterdeckset.InitialState(), firstSetAt, waterWind, fireEarth)

	require.Equal(t, []starterdeckset.Event{&starterdeckset.DecksSetEvent{
		EventMeta: starterdeckset.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: firstSetAt},
		PublicID:  publicID,
		Decks:     []starterdeckset.Deck{starterdecksettest.Deck(t, fireEarth, rules), starterdecksettest.Deck(t, waterWind, rules)},
		SetBy:     f.operator,
	}}, events)
	set := state.(starterdeckset.StarterDeckSet)
	require.Equal(t, int64(1), set.Version)
	require.Equal(t, publicID, set.PublicID)
	require.Equal(t, firstSetAt, set.SetAt)
}

func TestDecide_ResetReplacesDecksWithNextVersion(t *testing.T) {
	f := newFixture(t)
	state, _ := f.set(t, starterdeckset.InitialState(), firstSetAt, f.fireEarth(t, 7), f.waterWind(t, 3))

	state, events := f.set(t, state, resetAt, f.fireEarth(t, 7), f.lightDark(t))

	require.Len(t, events, 1)
	set := state.(starterdeckset.StarterDeckSet)
	require.Equal(t, int64(2), set.Version)
	require.Equal(t, []starterdeckset.Deck{starterdecksettest.Deck(t, f.fireEarth(t, 7), rules), starterdecksettest.Deck(t, f.lightDark(t), rules)}, set.Decks)
	require.Equal(t, resetAt, set.SetAt)
}

// 写す中身が全部同じなら版を進めない。同じプリセットデッキでも、直されていれば写し直す（反映する）
func TestDecide_EmitsOnlyWhenCopiesChange(t *testing.T) {
	f := newFixture(t)
	state, _ := f.set(t, starterdeckset.InitialState(), firstSetAt, f.fireEarth(t, 7), f.waterWind(t, 3))
	tests := []struct {
		name  string
		decks []presetdeck.PresetDeck
		want  int
	}{
		{name: "same_copies_in_other_order", decks: []presetdeck.PresetDeck{f.waterWind(t, 3), f.fireEarth(t, 7)}, want: 0},
		{name: "same_preset_deck_changed_since_copied", decks: []presetdeck.PresetDeck{f.fireEarth(t, 8), f.waterWind(t, 3)}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := starterdeckset.Decide(state, f.setCommand(t, tt.decks...), resetAt)

			require.NoError(t, err)
			require.Len(t, events, tt.want)
		})
	}
}

func TestDecide_RejectsInvalidDecks(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name  string
		decks func() []presetdeck.PresetDeck
		want  error
	}{
		{name: "one_deck", decks: func() []presetdeck.PresetDeck {
			return []presetdeck.PresetDeck{f.fireEarth(t, 1)}
		}, want: starterdeckset.ErrDeckCountMismatch},
		{name: "three_decks", decks: func() []presetdeck.PresetDeck {
			return []presetdeck.PresetDeck{f.fireEarth(t, 1), f.waterWind(t, 1), f.lightDark(t)}
		}, want: starterdeckset.ErrDeckCountMismatch},
		{name: "same_preset_deck_twice", decks: func() []presetdeck.PresetDeck {
			return []presetdeck.PresetDeck{f.fireEarth(t, 1), f.fireEarth(t, 1)}
		}, want: starterdeckset.ErrDuplicatedDeck},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := starterdeckset.Decide(starterdeckset.InitialState(), f.setCommand(t, tt.decks()...), firstSetAt)

			require.ErrorIs(t, err, tt.want)
		})
	}
}

// 2 つのスターターデッキに同じカードが入っていても、参照は 1 つにする
func TestStarterDeckSet_CardsAreDistinct(t *testing.T) {
	f := newFixture(t)
	state, _ := f.set(t, starterdeckset.InitialState(), firstSetAt, f.fireEarth(t, 7), f.waterWind(t, 3))

	cards := state.(starterdeckset.StarterDeckSet).Cards()

	// 共通の 9 種類と、それぞれの最後の 1 種類
	require.Len(t, cards, rules.TotalCardCount/rules.MaxCardDuplicates+1)
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	f := newFixture(t)
	meta := starterdeckset.EventMeta{EntityID: f.id, AggregateVersion: 1, OccurredAt: firstSetAt}
	event := &starterdeckset.DecksSetEvent{
		EventMeta: meta,
		PublicID:  publicID,
		Decks:     []starterdeckset.Deck{starterdecksettest.Deck(t, f.fireEarth(t, 7), rules), starterdecksettest.Deck(t, f.waterWind(t, 3), rules)},
		SetBy:     f.operator,
	}
	require.Equal(t, 1, event.SchemaVersion())
	golden, err := os.ReadFile(filepath.Join("testdata", "events", "DecksSetEvent.v1.json"))
	require.NoError(t, err)

	payload, err := json.Marshal(event)

	require.NoError(t, err)
	require.JSONEq(t, string(golden), string(payload))
	restored, err := starterdeckset.NewEventType(event.EventType()).Unmarshal(meta, event.SchemaVersion(), golden)
	require.NoError(t, err)
	var want starterdeckset.Event = event
	require.Equal(t, want, restored)
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := starterdeckset.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(starterdeckset.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}
