package studiorules_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules/studiorulestest"
)

var (
	createdAt = time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	publicID  = uuid.MustParse("00000000-0000-0000-0000-0000000000a1")
)

func createCommand(t *testing.T) *studiorules.CreateCommand {
	t.Helper()
	return &studiorules.CreateCommand{ID: studiorulestest.ID(t, 7), PublicID: publicID, Rules: studiorules.Defaults()}
}

func create(t *testing.T) (studiorules.State, studiorules.Event) {
	t.Helper()
	events, err := studiorules.Decide(studiorules.InitialState(), createCommand(t), createdAt)
	require.NoError(t, err)
	require.Len(t, events, 1)
	state, err := studiorules.Evolve(studiorules.InitialState(), events[0])
	require.NoError(t, err)
	return state, events[0]
}

func TestDefaults_HaveCurrentValues(t *testing.T) {
	defaults := studiorules.Defaults()

	require.Equal(t, 1, defaults.Board.MinSize.Int())
	require.Equal(t, 32, defaults.Board.MaxSize.Int())
}

func TestDecide_CreateEmitsCreatedEvent(t *testing.T) {
	cmd := createCommand(t)

	_, event := create(t)

	require.Equal(t, &studiorules.CreatedEvent{
		EventMeta: studiorules.EventMeta{EntityID: cmd.ID, AggregateVersion: 1, OccurredAt: createdAt},
		PublicID:  publicID,
		Rules:     cmd.Rules,
	}, event)
}

func TestDecide_RejectsCreateWhenAlreadyCreated(t *testing.T) {
	created, _ := create(t)

	_, err := studiorules.Decide(created, createCommand(t), createdAt)

	require.ErrorIs(t, err, studiorules.ErrAlreadyCreated)
}

func TestEvolve_CreatedEventMakesStudioRules(t *testing.T) {
	cmd := createCommand(t)

	state, _ := create(t)

	require.Equal(t, studiorules.StudioRules{ID: cmd.ID, Version: 1, PublicID: publicID, Rules: cmd.Rules}, state)
}

func TestEvolve_RejectsCreatedEventWhenAlreadyCreated(t *testing.T) {
	created, event := create(t)

	_, err := studiorules.Evolve(created, event)

	require.ErrorIs(t, err, studiorules.ErrInvalidState)
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

	restored, err := studiorules.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
	require.NoError(t, err)
	require.Equal(t, event, restored)
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := studiorules.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(studiorules.EventMeta{}, 1, []byte(`{}`))

	require.Error(t, err)
}
