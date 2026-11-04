package gamerules_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules/gamerulestest"
)

var (
	createdAt = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	publicID  = uuid.MustParse("00000000-0000-0000-0000-0000000000a1")
)

func createCommand(t *testing.T) *gamerules.CreateCommand {
	t.Helper()
	return &gamerules.CreateCommand{ID: gamerulestest.ID(t, 7), PublicID: publicID, Rules: gamerules.Defaults()}
}

func create(t *testing.T) (gamerules.State, gamerules.Event) {
	t.Helper()
	events, err := gamerules.Decide(gamerules.InitialState(), createCommand(t), createdAt)
	require.NoError(t, err)
	require.Len(t, events, 1)
	state, err := gamerules.Evolve(gamerules.InitialState(), events[0])
	require.NoError(t, err)
	return state, events[0]
}

func TestDefaults_AreValid(t *testing.T) {
	defaults := gamerules.Defaults()

	got, err := gamerules.New(defaults.Player, defaults.Deck, defaults.Match)

	require.NoError(t, err)
	require.Equal(t, defaults, got)
}

func TestDefaults_HaveCurrentValues(t *testing.T) {
	defaults := gamerules.Defaults()

	require.Equal(t, 40, defaults.Deck.TotalCardCount.Int())
	require.Equal(t, 4, defaults.Deck.MaxCardDuplicates.Int())
	require.Equal(t, 10, defaults.Player.MaxDecks.Int())
	require.Equal(t, 5, defaults.Player.MaxAvatars.Int())
	require.Equal(t, 2, defaults.Match.MinPlayers.Int())
	require.Equal(t, 4, defaults.Match.MaxPlayers.Int())
	require.Equal(t, 6, defaults.Match.MaxHandCards.Int())
	require.Equal(t, 5, defaults.Match.StartingHandCards.Int())
}

func TestNew_RejectsMinPlayersAboveMaxPlayers(t *testing.T) {
	rules := gamerules.Defaults()
	rules.Match = gamerulestest.Match(t, 5, 4, 6, 5)

	_, err := gamerules.New(rules.Player, rules.Deck, rules.Match)

	require.ErrorIs(t, err, gamerules.ErrInvalidRules)
}

func TestNew_RejectsStartingHandCardsAboveMaxHandCards(t *testing.T) {
	rules := gamerules.Defaults()
	rules.Match = gamerulestest.Match(t, 2, 4, 6, 7)

	_, err := gamerules.New(rules.Player, rules.Deck, rules.Match)

	require.ErrorIs(t, err, gamerules.ErrInvalidRules)
}

func TestNew_AcceptsStartingHandCardsEqualToMaxHandCards(t *testing.T) {
	rules := gamerules.Defaults()
	rules.Match = gamerulestest.Match(t, 2, 4, 6, 6)

	_, err := gamerules.New(rules.Player, rules.Deck, rules.Match)

	require.NoError(t, err)
}

func TestNew_RejectsStartingHandCardsAboveDeckTotalCardCount(t *testing.T) {
	rules := gamerules.Defaults()
	rules.Match = gamerulestest.Match(t, 2, 4, 50, 41)

	_, err := gamerules.New(rules.Player, rules.Deck, rules.Match)

	require.ErrorIs(t, err, gamerules.ErrInvalidRules)
}

func TestNew_AcceptsMinPlayersEqualToMaxPlayers(t *testing.T) {
	rules := gamerules.Defaults()
	rules.Match = gamerulestest.Match(t, 3, 3, 6, 5)

	_, err := gamerules.New(rules.Player, rules.Deck, rules.Match)

	require.NoError(t, err)
}

func TestDecide_CreateEmitsCreatedEvent(t *testing.T) {
	cmd := createCommand(t)

	_, event := create(t)

	require.Equal(t, &gamerules.CreatedEvent{
		EventMeta: gamerules.EventMeta{EntityID: cmd.ID, AggregateVersion: 1, OccurredAt: createdAt},
		PublicID:  publicID,
		Rules:     cmd.Rules,
	}, event)
}

func TestDecide_RejectsCreateWhenAlreadyCreated(t *testing.T) {
	created, _ := create(t)

	_, err := gamerules.Decide(created, createCommand(t), createdAt)

	require.ErrorIs(t, err, gamerules.ErrAlreadyCreated)
}

func TestEvolve_CreatedEventMakesGameRules(t *testing.T) {
	cmd := createCommand(t)

	state, _ := create(t)

	require.Equal(t, gamerules.GameRules{ID: cmd.ID, Version: 1, PublicID: publicID, Rules: cmd.Rules}, state)
}

func TestEvolve_RejectsCreatedEventWhenAlreadyCreated(t *testing.T) {
	created, event := create(t)

	_, err := gamerules.Evolve(created, event)

	require.ErrorIs(t, err, gamerules.ErrInvalidState)
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

	restored, err := gamerules.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
	require.NoError(t, err)
	require.Equal(t, event, restored)
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := gamerules.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(gamerules.EventMeta{}, 1, []byte(`{}`))

	require.Error(t, err)
}
