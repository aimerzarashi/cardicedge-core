package deck_test

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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck/decktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
)

var (
	createdAt = time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	cardA     = uuid.MustParse("00000000-0000-0000-0000-000000000001")
)

func createCommand(t *testing.T) *deck.CreateCommand {
	t.Helper()
	return &deck.CreateCommand{
		ID:           decktest.ID(t, 40),
		Player:       playertest.ID(t, 10),
		Name:         decktest.Name(t, "火"),
		Cards:        []card.Count{cardtest.Count(t, cardA, 4)},
		StarterGrant: startergranttest.ID(t, 20),
		Number:       startergranttest.DeckNumber(t, 1),
	}
}

func create(t *testing.T) (deck.State, deck.Event) {
	t.Helper()
	events, err := deck.Decide(deck.InitialState(), createCommand(t), createdAt)
	require.NoError(t, err)
	require.Len(t, events, 1)
	state, err := deck.Evolve(deck.InitialState(), events[0])
	require.NoError(t, err)
	return state, events[0]
}

func TestDecide_CreateEmitsCreatedEvent(t *testing.T) {
	cmd := createCommand(t)

	_, event := create(t)

	require.Equal(t, &deck.CreatedEvent{
		EventMeta:    deck.EventMeta{EntityID: cmd.ID, AggregateVersion: 1, OccurredAt: createdAt},
		Player:       cmd.Player,
		Name:         cmd.Name,
		Cards:        cmd.Cards,
		StarterGrant: cmd.StarterGrant,
		Number:       cmd.Number,
	}, event)
}

func TestDecide_RejectsCreateWhenAlreadyCreated(t *testing.T) {
	created, _ := create(t)

	_, err := deck.Decide(created, createCommand(t), createdAt)

	require.ErrorIs(t, err, deck.ErrAlreadyCreated)
}

func TestEvolve_CreatedEventMakesDeck(t *testing.T) {
	cmd := createCommand(t)

	state, _ := create(t)

	require.Equal(t, deck.Deck{ID: cmd.ID, Version: 1}, state)
}

func TestEvolve_RejectsCreatedEventWhenAlreadyCreated(t *testing.T) {
	created, event := create(t)

	_, err := deck.Evolve(created, event)

	require.ErrorIs(t, err, deck.ErrInvalidState)
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	_, event := create(t)
	require.Equal(t, 1, event.SchemaVersion())
	golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
	require.NoError(t, err)

	payload, err := json.Marshal(event)
	require.NoError(t, err)
	require.JSONEq(t, string(golden), string(payload))

	restored, err := deck.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
	require.NoError(t, err)
	require.Equal(t, event, restored)
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := deck.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(deck.EventMeta{}, 1, []byte(`{}`))

	require.Error(t, err)
}

func TestStarterKey_DiffersPerGrantAndNumber(t *testing.T) {
	starterGrantID := startergranttest.ID(t, 20)
	first := deck.StarterKey(starterGrantID, startergranttest.DeckNumber(t, 1))
	second := deck.StarterKey(starterGrantID, startergranttest.DeckNumber(t, 2))
	other := deck.StarterKey(startergranttest.ID(t, 21), startergranttest.DeckNumber(t, 1))

	require.NotEqual(t, first, second)
	require.NotEqual(t, first, other)
	require.Equal(t, deck.StarterKeyType, first.Type())
}

func TestNameKey_UsesNormalizedNameOfPlayer(t *testing.T) {
	full := deck.NameKey(playertest.ID(t, 10), decktest.Name(t, "ＡＢＣ"))
	half := deck.NameKey(playertest.ID(t, 10), decktest.Name(t, " abc "))

	require.Equal(t, full, half)
	require.Equal(t, deck.NameKeyType, full.Type())
}

func TestNameKey_DiffersPerPlayer(t *testing.T) {
	require.NotEqual(t,
		deck.NameKey(playertest.ID(t, 1), decktest.Name(t, "abc")),
		deck.NameKey(playertest.ID(t, 2), decktest.Name(t, "abc")),
	)
}

// 持ち主の ID と名前の数字がつながっても、別の組と同じ key にならない
func TestNameKey_DistinguishesHowPlayerAndNameAreSplit(t *testing.T) {
	require.NotEqual(t,
		deck.NameKey(playertest.ID(t, 1), decktest.Name(t, "23")),
		deck.NameKey(playertest.ID(t, 12), decktest.Name(t, "3")),
	)
}

func TestPlayerQuota_CountsPerPlayer(t *testing.T) {
	quota := deck.PlayerQuota(playertest.ID(t, 10))

	require.Equal(t, deck.PlayerQuotaType, quota.Type())
	require.Equal(t, int64(10), quota.OwnerID())
}
