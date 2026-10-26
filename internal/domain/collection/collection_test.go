package collection_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection/collectiontest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
)

var (
	createdAt = time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	cardA     = uuid.MustParse("00000000-0000-0000-0000-000000000001")
)

func createCommand(t *testing.T) *collection.CreateCommand {
	t.Helper()
	return &collection.CreateCommand{
		ID:           collectiontest.ID(t, 30),
		Player:       playertest.ID(t, 10),
		Cards:        []startergrant.CardCount{startergranttest.CardCount(t, cardA, 4)},
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

	require.Equal(t, collection.Collection{ID: cmd.ID, Version: 1, Player: cmd.Player, Cards: cmd.Cards, StarterGrant: cmd.StarterGrant}, state)
}

func TestEvolve_RejectsCreatedEventWhenAlreadyCreated(t *testing.T) {
	created, event := create(t)

	_, err := collection.Evolve(created, event)

	require.ErrorIs(t, err, collection.ErrInvalidState)
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

	restored, err := collection.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
	require.NoError(t, err)
	require.Equal(t, event, restored)
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := collection.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(collection.EventMeta{}, 1, []byte(`{}`))

	require.Error(t, err)
}

func TestPlayerKey_DiffersPerPlayer(t *testing.T) {
	require.NotEqual(t, collection.PlayerKey(playertest.ID(t, 10)), collection.PlayerKey(playertest.ID(t, 11)))
}
