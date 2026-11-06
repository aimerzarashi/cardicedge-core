package collection_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection/collectiontest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match/matchtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
)

var createdAt = time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)

func cardA(t *testing.T) card.Ref {
	t.Helper()
	return cardtest.CreatureRef(t, 1)
}

func createCommand(t *testing.T) *collection.CreateCommand {
	t.Helper()
	return &collection.CreateCommand{
		ID:           collectiontest.ID(t, 30),
		Player:       playertest.ID(t, 10),
		Cards:        []collection.Card{collectiontest.Card(t, cardA(t), 4)},
		StarterGrant: startergranttest.ID(t, 20),
	}
}

func create(t *testing.T) (collection.State, collection.Event) {
	t.Helper()
	events, err := collection.Decide(collection.InitialState(), createCommand(t), createdAt)
	require.NoError(t, err)
	require.Len(t, events, 1)
	state, err := collection.Evolve(collection.InitialState(), events[0])
	require.NoError(t, err)
	return state, events[0]
}

func TestDecide_CreateEmitsCreatedEvent(t *testing.T) {
	cmd := createCommand(t)

	_, event := create(t)

	require.Equal(t, &collection.CreatedEvent{
		EventMeta:    collection.EventMeta{EntityID: cmd.ID, AggregateVersion: 1, OccurredAt: createdAt},
		Player:       cmd.Player,
		Cards:        cmd.Cards,
		StarterGrant: cmd.StarterGrant,
	}, event)
}

func TestDecide_RejectsCreateWhenAlreadyCreated(t *testing.T) {
	created, _ := create(t)

	_, err := collection.Decide(created, createCommand(t), createdAt)

	require.ErrorIs(t, err, collection.ErrAlreadyCreated)
}

func TestEvolve_CreatedEventMakesCollection(t *testing.T) {
	cmd := createCommand(t)

	state, _ := create(t)

	require.Equal(t, collection.Collection{ID: cmd.ID, Version: 1}, state)
}

func TestEvolve_RejectsCreatedEventWhenAlreadyCreated(t *testing.T) {
	created, event := create(t)

	_, err := collection.Evolve(created, event)

	require.ErrorIs(t, err, collection.ErrInvalidState)
}

func acquireCommand(t *testing.T, matchID int64) *collection.AcquireCommand {
	t.Helper()
	return &collection.AcquireCommand{
		Cards:  []collection.Card{collectiontest.Card(t, cardA(t), 1)},
		Source: collection.NewMatchRewardSource(matchtest.ID(t, matchID)),
	}
}

func acquire(t *testing.T, state collection.State, matchID int64) (collection.State, collection.Event) {
	t.Helper()
	events, err := collection.Decide(state, acquireCommand(t, matchID), createdAt)
	require.NoError(t, err)
	require.Len(t, events, 1)
	next, err := collection.Evolve(state, events[0])
	require.NoError(t, err)
	return next, events[0]
}

func TestDecide_AcquireEmitsCardsAcquiredEventWithItsSource(t *testing.T) {
	created, _ := create(t)
	cmd := acquireCommand(t, 40)

	_, event := acquire(t, created, 40)

	require.Equal(t, &collection.CardsAcquiredEvent{
		EventMeta: collection.EventMeta{EntityID: collectiontest.ID(t, 30), AggregateVersion: 2, OccurredAt: createdAt},
		Cards:     cmd.Cards,
		Source:    cmd.Source,
	}, event)
}

func TestDecide_AcquireRejectsTheSameMatchRewardTwice(t *testing.T) {
	created, _ := create(t)
	acquired, _ := acquire(t, created, 40)

	_, err := collection.Decide(acquired, acquireCommand(t, 40), createdAt)

	require.ErrorIs(t, err, collection.ErrAlreadyAcquired)
}

func TestDecide_AcquireAcceptsTheRewardOfAnotherMatch(t *testing.T) {
	created, _ := create(t)
	acquired, _ := acquire(t, created, 40)

	next, event := acquire(t, acquired, 41)

	require.Equal(t, int64(3), event.GetMeta().AggregateVersion)
	require.Equal(t, []match.ID{matchtest.ID(t, 40), matchtest.ID(t, 41)}, next.(collection.Collection).RewardedMatches)
}

func TestDecide_AcquireRejectsNoCardsAndNoSource(t *testing.T) {
	created, _ := create(t)
	tests := map[string]*collection.AcquireCommand{
		"no_cards":  {Source: collection.NewMatchRewardSource(matchtest.ID(t, 40))},
		"no_source": {Cards: []collection.Card{collectiontest.Card(t, cardA(t), 1)}},
	}
	for name, cmd := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := collection.Decide(created, cmd, createdAt)

			require.ErrorIs(t, err, collection.ErrInvalidAcquisition)
		})
	}
}

func TestDecide_AcquireRejectsUncreatedCollection(t *testing.T) {
	_, err := collection.Decide(collection.InitialState(), acquireCommand(t, 40), createdAt)

	require.ErrorIs(t, err, collection.ErrInvalidState)
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	created, createdEvent := create(t)
	_, acquiredEvent := acquire(t, created, 40)
	for _, event := range []collection.Event{createdEvent, acquiredEvent} {
		t.Run(event.EventType().String(), func(t *testing.T) {
			require.Equal(t, 1, event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := collection.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, event, restored)
		})
	}
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := collection.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(collection.EventMeta{}, 1, []byte(`{}`))

	require.Error(t, err)
}

func TestPlayerKey_DiffersPerPlayer(t *testing.T) {
	require.NotEqual(t, collection.PlayerKey(playertest.ID(t, 10)), collection.PlayerKey(playertest.ID(t, 11)))
}
