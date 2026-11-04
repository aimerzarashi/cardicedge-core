package match_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar/avatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck/decktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match/matchtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar/npcavatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck/npcdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
)

var startedAt = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

// 拠点 (0,0) と火属性の土地 (1,0) がつながった 2×1 のボード
func twoTileBoard(t *testing.T) match.Board {
	t.Helper()
	return match.Board{
		ID:          boardtest.ID(t, 5),
		Size:        boardtest.Size(t, 2, 1),
		Tiles:       []board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "land", "fire")},
		Connections: []board.Connection{boardtest.Connection(t, 0, 0, 1, 0)},
	}
}

func deckCard(t *testing.T, id int64, quantity int) match.DeckCard {
	t.Helper()
	return match.DeckCard{Card: cardtest.CreatureRef(t, id), Quantity: cardtest.DeckQuantity(t, quantity)}
}

// プレイヤー（カード 101 が 2 枚・102 が 1 枚）
func playerParticipant(t *testing.T) match.Participant {
	t.Helper()
	return match.NewPlayerParticipant(playertest.ID(t, 10), avatartest.ID(t, 1), decktest.ID(t, 2), []match.DeckCard{deckCard(t, 101, 2), deckCard(t, 102, 1)})
}

// NPC（カード 201 が 3 枚）
func npcParticipant(t *testing.T) match.Participant {
	t.Helper()
	return match.NewNPCParticipant(npcavatartest.ID(t, 7), npcdecktest.ID(t, 8), []match.DeckCard{deckCard(t, 201, 3)})
}

func startCommand(t *testing.T, rules match.Rules, participants ...match.Participant) *match.StartCommand {
	t.Helper()
	return &match.StartCommand{ID: matchtest.ID(t, 40), Participants: participants, Board: twoTileBoard(t), Rules: rules}
}

func start(t *testing.T, cmd *match.StartCommand) (match.Match, []match.Event) {
	t.Helper()
	events, err := match.Decide(match.InitialState(), cmd, startedAt, matchtest.NewRandom())
	require.NoError(t, err)
	state := match.InitialState()
	for _, event := range events {
		state, err = match.Evolve(state, event)
		require.NoError(t, err)
	}
	current, ok := state.(match.Match)
	require.True(t, ok)
	return current, events
}

func indexes(t testing.TB, values ...int) []match.ParticipantIndex {
	result := make([]match.ParticipantIndex, len(values))
	for i, value := range values {
		result[i] = matchtest.Index(t, value)
	}
	return result
}

func refs(t *testing.T, ids ...int64) []card.Ref {
	t.Helper()
	result := make([]card.Ref, len(ids))
	for i, id := range ids {
		result[i] = cardtest.CreatureRef(t, id)
	}
	return result
}

func TestDecide_StartPutsEveryoneOnBaseWithStartingMagicAtRoundOne(t *testing.T) {
	cmd := startCommand(t, matchtest.Rules(t, 1, 6, 2), playerParticipant(t), npcParticipant(t))

	state, events := start(t, cmd)

	require.Len(t, events, 4)
	require.Equal(t, int64(4), state.Version)
	require.Equal(t, 1, state.Round)
	require.Equal(t, cmd.Board, state.Board)
	require.Equal(t, cmd.Rules, state.Rules)
	for i, participant := range state.Participants {
		require.Equal(t, cmd.Participants[i], participant.Participant)
		require.Equal(t, boardtest.Position(t, 0, 0), participant.Position)
		require.Equal(t, 600, participant.Magic)
	}
}

// 順番は参加者の並びを乱数の元で混ぜたもの（テストの乱数の元は逆順にする）
func TestDecide_StartShufflesParticipantsIntoTheTurnOrderAndGivesTheTurnToTheFirst(t *testing.T) {
	cmd := startCommand(t, matchtest.Rules(t, 1, 6, 1), playerParticipant(t), npcParticipant(t), npcParticipant(t))

	state, _ := start(t, cmd)

	require.Equal(t, indexes(t, 2, 1, 0), state.TurnOrder)
	current, ok := state.CurrentParticipant()
	require.True(t, ok)
	require.Equal(t, matchtest.Index(t, 2), current)
}

func TestDecide_StartShufflesEachDeckAndDealsTheStartingHandFromTheTop(t *testing.T) {
	cmd := startCommand(t, matchtest.Rules(t, 1, 6, 2), playerParticipant(t), npcParticipant(t))

	state, _ := start(t, cmd)

	// 並べると 101, 101, 102。混ぜる（テストの乱数の元は逆順）と 102, 101, 101 で、先頭の 2 枚を配る
	require.Equal(t, refs(t, 102, 101), state.Participants[0].Hand)
	require.Equal(t, refs(t, 101), state.Participants[0].DrawPile)
	require.Equal(t, refs(t, 201, 201), state.Participants[1].Hand)
	require.Equal(t, refs(t, 201), state.Participants[1].DrawPile)
}

func TestDecide_StartDealsOnlyWhatTheDeckHasWhenTheDeckIsSmallerThanTheStartingHand(t *testing.T) {
	cmd := startCommand(t, matchtest.Rules(t, 1, 6, 5), playerParticipant(t), npcParticipant(t))

	state, _ := start(t, cmd)

	require.Len(t, state.Participants[0].Hand, 3)
	require.Empty(t, state.Participants[0].DrawPile)
}

func TestDecide_StartRejectsBoardWithoutBase(t *testing.T) {
	cmd := startCommand(t, matchtest.Rules(t, 1, 6, 2), playerParticipant(t), npcParticipant(t))
	cmd.Board.Tiles = []board.Tile{boardtest.Tile(t, 0, 0, "land", "fire")}

	_, err := match.Decide(match.InitialState(), cmd, startedAt, matchtest.NewRandom())

	require.ErrorIs(t, err, match.ErrBaseNotFound)
}

func TestDecide_RejectsStartWhenAlreadyCreated(t *testing.T) {
	cmd := startCommand(t, matchtest.Rules(t, 1, 6, 2), playerParticipant(t), npcParticipant(t))
	state, _ := start(t, cmd)

	_, err := match.Decide(state, cmd, startedAt, matchtest.NewRandom())

	require.ErrorIs(t, err, match.ErrAlreadyCreated)
}

func TestEvolve_RejectsEventsBeforeCreatedAndSecondCreated(t *testing.T) {
	_, err := match.Evolve(match.InitialState(), &match.HandsDealtEvent{Cards: 1})
	require.ErrorIs(t, err, match.ErrInvalidState)

	cmd := startCommand(t, matchtest.Rules(t, 1, 6, 2), playerParticipant(t), npcParticipant(t))
	state, events := start(t, cmd)
	_, err = match.Evolve(state, events[0])
	require.ErrorIs(t, err, match.ErrInvalidState)
}

func TestEvolve_DoesNotChangeGivenState(t *testing.T) {
	cmd := startCommand(t, matchtest.Rules(t, 1, 6, 2), playerParticipant(t), npcParticipant(t))
	events, err := match.Decide(match.InitialState(), cmd, startedAt, matchtest.NewRandom())
	require.NoError(t, err)
	state := match.InitialState()
	for _, event := range events[:3] {
		state, err = match.Evolve(state, event)
		require.NoError(t, err)
	}
	before := state.(match.Match)
	pile := append([]card.Ref(nil), before.Participants[0].DrawPile...)

	_, err = match.Evolve(state, events[3])

	require.NoError(t, err)
	require.Equal(t, pile, before.Participants[0].DrawPile)
	require.Empty(t, before.Participants[0].Hand)
}

func TestMatch_HasPlayerIsTheParticipatingPlayersOnly(t *testing.T) {
	cmd := startCommand(t, matchtest.Rules(t, 1, 6, 2), playerParticipant(t), npcParticipant(t))
	state, _ := start(t, cmd)

	require.True(t, state.HasPlayer(playertest.ID(t, 10)))
	require.False(t, state.HasPlayer(playertest.ID(t, 11)))
}

// 保存済みのイベントを読めなくなる変更（payload の形の変更）を検出する。形を変えるときは版を上げ、古い版の golden も残す
func TestEvent_PayloadMatchesGoldenV1(t *testing.T) {
	meta := match.EventMeta{EntityID: matchtest.ID(t, 40), AggregateVersion: 2, OccurredAt: startedAt}
	events := []match.Event{
		&match.CreatedEvent{EventMeta: meta, Participants: []match.Participant{playerParticipant(t), npcParticipant(t)}, Board: twoTileBoard(t), Rules: matchtest.Rules(t, 1, 6, 5)},
		&match.TurnOrderDecidedEvent{EventMeta: meta, Order: indexes(t, 1, 0)},
		&match.DecksShuffledEvent{EventMeta: meta, DrawPiles: [][]card.Ref{refs(t, 102, 101, 101), refs(t, 201, 201, 201)}},
		&match.HandsDealtEvent{EventMeta: meta, Cards: 5},
	}
	for _, event := range events {
		t.Run(event.EventType().String(), func(t *testing.T) {
			require.Equal(t, 1, event.SchemaVersion())
			golden, err := os.ReadFile(filepath.Join("testdata", "events", event.EventType().String()+".v1.json"))
			require.NoError(t, err)

			payload, err := json.Marshal(event)
			require.NoError(t, err)
			require.JSONEq(t, string(golden), string(payload))

			restored, err := match.NewEventType(event.EventType()).Unmarshal(event.GetMeta(), event.SchemaVersion(), golden)
			require.NoError(t, err)
			require.Equal(t, event, restored)
		})
	}
}

func TestUnmarshal_RejectsUnknownEventType(t *testing.T) {
	_, err := match.NewEventType(domain.NewEventName("UnknownEvent")).Unmarshal(match.EventMeta{}, 1, []byte(`{}`))

	require.Error(t, err)
}
