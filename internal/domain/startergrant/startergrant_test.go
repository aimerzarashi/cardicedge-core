package startergrant_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
)

var (
	startedAt = time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	cardA     = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	cardB     = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	cardC     = uuid.MustParse("00000000-0000-0000-0000-000000000003")
)

func decks(t *testing.T) []startergrant.Deck {
	t.Helper()
	return []startergrant.Deck{
		startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4), startergranttest.CardCount(t, cardB, 1)),
		startergranttest.Deck(t, "水", startergranttest.CardCount(t, cardA, 2), startergranttest.CardCount(t, cardB, 3), startergranttest.CardCount(t, cardC, 2)),
	}
}

func startCommand(t *testing.T) *startergrant.StartCommand {
	t.Helper()
	return &startergrant.StartCommand{ID: startergranttest.ID(t, 20), Player: playertest.ID(t, 10), Decks: decks(t)}
}

// 状態に畳み込んで、イベントを返す
func apply(t *testing.T, state startergrant.State, cmd startergrant.Command) (startergrant.State, []startergrant.Event) {
	t.Helper()
	events, err := startergrant.Decide(state, cmd, startedAt)
	require.NoError(t, err)
	for _, event := range events {
		state, err = startergrant.Evolve(state, event)
		require.NoError(t, err)
	}
	return state, events
}

func started(t *testing.T) (startergrant.State, startergrant.Event) {
	t.Helper()
	state, events := apply(t, startergrant.InitialState(), startCommand(t))
	require.Len(t, events, 1)
	return state, events[0]
}

func TestDecide_StartCopiesDecksAndCollectsMostCountOfEachCard(t *testing.T) {
	_, event := started(t)

	started, ok := event.(*startergrant.StartedEvent)
	require.True(t, ok)
	require.Equal(t, []startergrant.CardCount{
		startergranttest.CardCount(t, cardA, 4),
		startergranttest.CardCount(t, cardB, 3),
		startergranttest.CardCount(t, cardC, 2),
	}, started.Cards)
	require.Equal(t, decks(t), started.Decks)
}

func TestDecide_RejectsStartWithoutDecks(t *testing.T) {
	cmd := startCommand(t)
	cmd.Decks = nil

	_, err := startergrant.Decide(startergrant.InitialState(), cmd, startedAt)

	require.ErrorIs(t, err, startergrant.ErrInvalidDeckCount)
}

// 配るデッキの数は公開中のスターターデッキの数で決まり、1 つでも、3 つ以上でも配れる。済む条件はその数すべての記録
func TestDecide_CompletesAfterRecordingEveryDeckWhateverTheDeckCount(t *testing.T) {
	tests := []struct {
		name  string
		count int
	}{
		{name: "one_deck", count: 1},
		{name: "three_decks", count: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := startCommand(t)
			cmd.Decks = nil
			for range tt.count {
				cmd.Decks = append(cmd.Decks, startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4)))
			}
			state, _ := apply(t, startergrant.InitialState(), cmd)
			state, _ = apply(t, state, &startergrant.RecordCollectionCommand{})
			for number := 1; number < tt.count; number++ {
				state, _ = apply(t, state, &startergrant.RecordDeckCommand{Number: startergranttest.DeckNumber(t, number)})
				require.IsType(t, startergrant.InProgressStarterGrant{}, state)
			}

			state, _ = apply(t, state, &startergrant.RecordDeckCommand{Number: startergranttest.DeckNumber(t, tt.count)})

			require.IsType(t, startergrant.CompletedStarterGrant{}, state)
		})
	}
}

func TestDecide_RejectsRecordDeckBeyondDeckCount(t *testing.T) {
	state, _ := started(t)

	_, err := startergrant.Decide(state, &startergrant.RecordDeckCommand{Number: startergranttest.DeckNumber(t, 3)}, startedAt)

	require.ErrorIs(t, err, startergrant.ErrInvalidDeckNumber)
}

func TestDecide_RejectsStartWhenAlreadyStarted(t *testing.T) {
	state, _ := started(t)

	_, err := startergrant.Decide(state, startCommand(t), startedAt)

	require.ErrorIs(t, err, startergrant.ErrAlreadyStarted)
}

func TestDecide_RejectsRecordBeforeStart(t *testing.T) {
	_, err := startergrant.Decide(startergrant.InitialState(), &startergrant.RecordCollectionCommand{}, startedAt)

	require.ErrorIs(t, err, startergrant.ErrInvalidState)
}

func TestDecide_RecordCollectionTwiceEmitsNoEventTheSecondTime(t *testing.T) {
	state, _ := started(t)
	state, first := apply(t, state, &startergrant.RecordCollectionCommand{})

	_, second := apply(t, state, &startergrant.RecordCollectionCommand{})

	require.Len(t, first, 1)
	require.Empty(t, second)
}

func TestDecide_RecordDeckTwiceEmitsNoEventTheSecondTime(t *testing.T) {
	state, _ := started(t)
	number := startergranttest.DeckNumber(t, 1)
	state, first := apply(t, state, &startergrant.RecordDeckCommand{Number: number})

	_, second := apply(t, state, &startergrant.RecordDeckCommand{Number: number})

	require.Len(t, first, 1)
	require.Empty(t, second)
}

func TestDecide_LastRecordEmitsRecordedAndCompletedEvents(t *testing.T) {
	state, _ := started(t)
	state, _ = apply(t, state, &startergrant.RecordDeckCommand{Number: startergranttest.DeckNumber(t, 2)})
	state, _ = apply(t, state, &startergrant.RecordCollectionCommand{})

	state, events := apply(t, state, &startergrant.RecordDeckCommand{Number: startergranttest.DeckNumber(t, 1)})

	require.Len(t, events, 2)
	require.IsType(t, &startergrant.DeckRecordedEvent{}, events[0])
	require.IsType(t, &startergrant.CompletedEvent{}, events[1])
	require.Equal(t, int64(5), events[1].GetMeta().AggregateVersion)
	require.Equal(t, startergrant.CompletedStarterGrant{ID: startergranttest.ID(t, 20), Version: 5}, state)
}

func TestDecide_RecordAfterCompletedEmitsNoEvent(t *testing.T) {
	state, _ := started(t)
	state, _ = apply(t, state, &startergrant.RecordCollectionCommand{})
	state, _ = apply(t, state, &startergrant.RecordDeckCommand{Number: startergranttest.DeckNumber(t, 1)})
	state, _ = apply(t, state, &startergrant.RecordDeckCommand{Number: startergranttest.DeckNumber(t, 2)})

	_, events := apply(t, state, &startergrant.RecordCollectionCommand{})

	require.Empty(t, events)
}

func TestEvolve_DoesNotChangeGivenState(t *testing.T) {
	state, _ := started(t)

	next, err := startergrant.Evolve(state, &startergrant.DeckRecordedEvent{
		EventMeta: startergrant.EventMeta{EntityID: startergranttest.ID(t, 20), AggregateVersion: 2, OccurredAt: startedAt},
		Number:    startergranttest.DeckNumber(t, 1),
	})

	require.NoError(t, err)
	require.False(t, state.(startergrant.InProgressStarterGrant).DeckRecorded[0])
	require.True(t, next.(startergrant.InProgressStarterGrant).DeckRecorded[0])
}

func TestEvolve_RejectsStartedEventWhenAlreadyStarted(t *testing.T) {
	state, event := started(t)

	_, err := startergrant.Evolve(state, event)

	require.ErrorIs(t, err, startergrant.ErrInvalidState)
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	meta := startergrant.EventMeta{EntityID: startergranttest.ID(t, 20), AggregateVersion: 1, OccurredAt: startedAt}
	_, started := started(t)
	events := []startergrant.Event{
		started,
		&startergrant.CollectionRecordedEvent{EventMeta: meta},
		&startergrant.DeckRecordedEvent{EventMeta: meta, Number: startergranttest.DeckNumber(t, 2)},
		&startergrant.CompletedEvent{EventMeta: meta},
	}
	for _, event := range events {
		t.Run(event.EventType().String(), func(t *testing.T) {
			require.Equal(t, 1, event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := startergrant.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, event, restored)
		})
	}
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := startergrant.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(startergrant.EventMeta{}, 1, []byte(`{}`))

	require.Error(t, err)
}

func TestNewDeckNumberAndCount_RejectOutOfRange(t *testing.T) {
	_, zero := startergrant.NewDeckNumber(0)
	_, negative := startergrant.NewDeckNumber(-1)
	_, noCount := startergrant.NewCount(0)

	require.ErrorIs(t, zero, startergrant.ErrInvalidDeckNumber)
	require.ErrorIs(t, negative, startergrant.ErrInvalidDeckNumber)
	require.ErrorIs(t, noCount, startergrant.ErrCountNotPositive)
}

func TestPlayerKey_UsesPlayerKeyType(t *testing.T) {
	key := startergrant.PlayerKey(playertest.ID(t, 10))

	require.Equal(t, startergrant.PlayerKeyType, key.Type())
	require.Equal(t, "10", key.Value())
}
