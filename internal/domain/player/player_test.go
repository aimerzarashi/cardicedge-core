package player_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
)

var (
	registeredAt = time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	publicID     = uuid.MustParse("11111111-2222-3333-4444-555555555555")
)

func registerCommand(t *testing.T) *player.RegisterCommand {
	t.Helper()
	return &player.RegisterCommand{
		ID:       playertest.ID(t, 10),
		PublicID: publicID,
		Issuer:   playertest.Issuer(t, "diecardomain-game"),
		Subject:  playertest.Subject(t, "user-1"),
	}
}

func register(t *testing.T) (player.State, player.Event) {
	t.Helper()
	events, err := player.Decide(player.InitialState(), registerCommand(t), registeredAt)
	require.NoError(t, err)
	require.Len(t, events, 1)
	state, err := player.Evolve(player.InitialState(), events[0])
	require.NoError(t, err)
	return state, events[0]
}

func TestDecide_RegisterEmitsRegisteredEvent(t *testing.T) {
	cmd := registerCommand(t)

	_, event := register(t)

	require.Equal(t, &player.RegisteredEvent{
		EventMeta: player.EventMeta{EntityID: cmd.ID, AggregateVersion: 1, OccurredAt: registeredAt},
		PublicID:  publicID,
		Issuer:    cmd.Issuer,
		Subject:   cmd.Subject,
	}, event)
}

func TestDecide_RejectsRegisterWhenAlreadyRegistered(t *testing.T) {
	registered, _ := register(t)

	_, err := player.Decide(registered, registerCommand(t), registeredAt)

	require.ErrorIs(t, err, player.ErrAlreadyRegistered)
}

func TestEvolve_RegisteredEventMakesPlayer(t *testing.T) {
	cmd := registerCommand(t)

	state, _ := register(t)

	require.Equal(t, player.Player{
		ID: cmd.ID, Version: 1, PublicID: publicID, Issuer: cmd.Issuer, Subject: cmd.Subject, RegisteredAt: registeredAt,
	}, state)
}

func TestEvolve_RejectsRegisteredEventWhenAlreadyRegistered(t *testing.T) {
	registered, event := register(t)

	_, err := player.Evolve(registered, event)

	require.ErrorIs(t, err, player.ErrInvalidState)
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	_, event := register(t)
	require.Equal(t, 1, event.SchemaVersion())
	golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
	require.NoError(t, err)

	payload, err := json.Marshal(event)
	require.NoError(t, err)
	require.JSONEq(t, string(golden), string(payload))

	restored, err := player.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
	require.NoError(t, err)
	require.Equal(t, event, restored)
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := player.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(player.EventMeta{}, 1, []byte(`{}`))
	require.Error(t, err)
}

func TestNewIssuerAndSubject_RejectBlank(t *testing.T) {
	_, issuerErr := player.NewIssuer(" ")
	_, subjectErr := player.NewSubject("")

	require.Error(t, issuerErr)
	require.Error(t, subjectErr)
}

// 区切り文字でつなぐと ("ab", "c") と ("a", "bc") が同じ key になり、別の組を同じ (iss, sub) として扱ってしまう
func TestSubjectKey_DistinguishesHowIssuerAndSubjectAreSplit(t *testing.T) {
	left := player.SubjectKey(playertest.Issuer(t, "ab"), playertest.Subject(t, "c"))
	right := player.SubjectKey(playertest.Issuer(t, "a"), playertest.Subject(t, "bc"))

	require.NotEqual(t, left, right)
	require.Equal(t, player.SubjectKeyType, left.Type())
}
