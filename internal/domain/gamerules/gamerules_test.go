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

	got, err := gamerules.New(defaults.Player, defaults.Deck, defaults.StarterDeck, defaults.Match)

	require.NoError(t, err)
	require.Equal(t, defaults, got)
}

func TestDefaults_HaveCurrentValues(t *testing.T) {
	defaults := gamerules.Defaults()

	require.Equal(t, 40, defaults.Deck.TotalCardCount)
	require.Equal(t, 4, defaults.Deck.MaxCardDuplicates)
	require.Equal(t, 2, defaults.StarterDeck.MaxPublished)
	require.Equal(t, 10, defaults.Player.MaxDecks)
	require.Equal(t, 5, defaults.Player.MaxAvatars)
	require.Equal(t, 2, defaults.Match.MinPlayers)
	require.Equal(t, 4, defaults.Match.MaxPlayers)
	require.Equal(t, 6, defaults.Match.MaxHandCards)
}

func TestNew_RejectsValueBelowOne(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*gamerules.Rules)
	}{
		{name: "zero_total_card_count", modify: func(r *gamerules.Rules) { r.Deck.TotalCardCount = 0 }},
		{name: "zero_max_card_duplicates", modify: func(r *gamerules.Rules) { r.Deck.MaxCardDuplicates = 0 }},
		{name: "zero_max_published", modify: func(r *gamerules.Rules) { r.StarterDeck.MaxPublished = 0 }},
		{name: "zero_max_decks", modify: func(r *gamerules.Rules) { r.Player.MaxDecks = 0 }},
		{name: "negative_max_avatars", modify: func(r *gamerules.Rules) { r.Player.MaxAvatars = -1 }},
		{name: "zero_min_players", modify: func(r *gamerules.Rules) { r.Match.MinPlayers = 0 }},
		{name: "zero_max_players", modify: func(r *gamerules.Rules) { r.Match.MaxPlayers = 0 }},
		{name: "zero_max_hand_cards", modify: func(r *gamerules.Rules) { r.Match.MaxHandCards = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := gamerules.Defaults()
			tt.modify(&rules)

			_, err := gamerules.New(rules.Player, rules.Deck, rules.StarterDeck, rules.Match)

			require.ErrorIs(t, err, gamerules.ErrInvalidRules)
		})
	}
}

func TestNew_RejectsMinPlayersAboveMaxPlayers(t *testing.T) {
	rules := gamerules.Defaults()
	rules.Match.MinPlayers = 5
	rules.Match.MaxPlayers = 4

	_, err := gamerules.New(rules.Player, rules.Deck, rules.StarterDeck, rules.Match)

	require.ErrorIs(t, err, gamerules.ErrInvalidRules)
}

func TestNew_AcceptsMinPlayersEqualToMaxPlayers(t *testing.T) {
	rules := gamerules.Defaults()
	rules.Match.MinPlayers = 3
	rules.Match.MaxPlayers = 3

	_, err := gamerules.New(rules.Player, rules.Deck, rules.StarterDeck, rules.Match)

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
