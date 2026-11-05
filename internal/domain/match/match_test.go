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
		&match.CardDrawnEvent{EventMeta: meta, Card: cardtest.CreatureRef(t, 101)},
		&match.CardDiscardedEvent{EventMeta: meta, HandIndex: 1, Card: cardtest.CreatureRef(t, 101)},
		&match.DiceRolledEvent{EventMeta: meta, Value: 4},
		&match.MovedEvent{EventMeta: meta, To: boardtest.Position(t, 1, 0)},
		&match.CheckpointPassedEvent{EventMeta: meta, Checkpoint: boardtest.Position(t, 1, 0), Magic: 100},
		&match.LapCompletedEvent{EventMeta: meta, Magic: 300},
		&match.TurnEndedEvent{EventMeta: meta},
		&match.MatchEndedEvent{EventMeta: meta, Winners: indexes(t, 0, 1), Reason: match.EndReasonRoundLimit},
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

// 拠点 (0,0) - チェックポイント (1,0) - 土地 (2,0) - 土地 (3,0)（行き止まり）の一本道
func lineBoard(t *testing.T) match.Board {
	t.Helper()
	return match.Board{
		ID:    boardtest.ID(t, 5),
		Size:  boardtest.Size(t, 4, 1),
		Tiles: []board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", ""), boardtest.Tile(t, 2, 0, "land", "fire"), boardtest.Tile(t, 3, 0, "land", "fire")},
		Connections: []board.Connection{
			boardtest.Connection(t, 0, 0, 1, 0), boardtest.Connection(t, 1, 0, 2, 0), boardtest.Connection(t, 2, 0, 3, 0),
		},
	}
}

// 拠点 (0,0) - チェックポイント (1,0) から、土地 (2,0)（行き止まり）とチェックポイント (1,1)（行き止まり）に分かれる
func forkBoard(t *testing.T) match.Board {
	t.Helper()
	return match.Board{
		ID:    boardtest.ID(t, 5),
		Size:  boardtest.Size(t, 3, 2),
		Tiles: []board.Tile{boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", ""), boardtest.Tile(t, 2, 0, "land", "fire"), boardtest.Tile(t, 1, 1, "checkpoint", "")},
		Connections: []board.Connection{
			boardtest.Connection(t, 0, 0, 1, 0), boardtest.Connection(t, 1, 0, 2, 0), boardtest.Connection(t, 1, 0, 1, 1),
		},
	}
}

// 拠点 (0,0) を含む 6 マスの輪。チェックポイントは (1,0) と (1,1)
func ringBoard(t *testing.T) match.Board {
	t.Helper()
	return match.Board{
		ID:   boardtest.ID(t, 5),
		Size: boardtest.Size(t, 3, 2),
		Tiles: []board.Tile{
			boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "checkpoint", ""), boardtest.Tile(t, 2, 0, "land", "fire"),
			boardtest.Tile(t, 2, 1, "land", "fire"), boardtest.Tile(t, 1, 1, "checkpoint", ""), boardtest.Tile(t, 0, 1, "land", "fire"),
		},
		Connections: []board.Connection{
			boardtest.Connection(t, 0, 0, 1, 0), boardtest.Connection(t, 1, 0, 2, 0), boardtest.Connection(t, 2, 0, 2, 1),
			boardtest.Connection(t, 2, 1, 1, 1), boardtest.Connection(t, 1, 1, 0, 1), boardtest.Connection(t, 0, 1, 0, 0),
		},
	}
}

func pos(t *testing.T, x, y int) board.Position {
	t.Helper()
	return boardtest.Position(t, x, y)
}

// 順番はテストの乱数の元で逆順になるので、最初の番は参加者 1（NPC）、次が参加者 0（プレイヤー）
func newMatch(t *testing.T, b match.Board, rules match.Rules) match.Match {
	t.Helper()
	cmd := startCommand(t, rules, playerParticipant(t), npcParticipant(t))
	cmd.Board = b
	state, _ := start(t, cmd)
	return state
}

func apply(t *testing.T, state match.Match, random match.Random, cmd match.Command) (match.Match, []match.Event) {
	t.Helper()
	events, err := match.Decide(state, cmd, startedAt, random)
	require.NoError(t, err)
	var next match.State = state
	for _, event := range events {
		next, err = match.Evolve(next, event)
		require.NoError(t, err)
	}
	return next.(match.Match), events
}

func decideErr(t *testing.T, state match.Match, cmd match.Command) error {
	t.Helper()
	_, err := match.Decide(state, cmd, startedAt, matchtest.NewRandom())
	return err
}

// 引いてから目 face を振る
func drawAndRoll(t *testing.T, state match.Match, by int, face int) (match.Match, []match.Event) {
	t.Helper()
	if state.Phase == match.PhaseDrawing {
		state, _ = apply(t, state, matchtest.NewRandom(), &match.DrawCommand{By: matchtest.Index(t, by)})
	}
	return apply(t, state, matchtest.NewRollingRandom(face), &match.RollCommand{By: matchtest.Index(t, by)})
}

func endTurn(t *testing.T, state match.Match, by int) (match.Match, []match.Event) {
	t.Helper()
	return apply(t, state, matchtest.NewRandom(), &match.EndTurnCommand{By: matchtest.Index(t, by)})
}

func choose(t *testing.T, state match.Match, by int, x, y int) (match.Match, []match.Event) {
	t.Helper()
	return apply(t, state, matchtest.NewRandom(), &match.ChoosePathCommand{By: matchtest.Index(t, by), To: pos(t, x, y)})
}

func countOf[T match.Event](events []match.Event) int {
	count := 0
	for _, event := range events {
		if _, ok := event.(T); ok {
			count++
		}
	}
	return count
}

func defaultRules(t *testing.T) match.Rules {
	t.Helper()
	return matchtest.Rules(t, 1, 6, 1)
}

func TestDecide_StartGivesTheFirstParticipantTheDrawPhase(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))

	require.Equal(t, match.PhaseDrawing, state.Phase)
	require.Zero(t, state.RemainingSteps)
	require.Zero(t, state.Dice)
	require.Nil(t, state.Result)
}

func TestDecide_StartSkipsTheDrawPhaseWhenTheFirstParticipantHasNoDrawPile(t *testing.T) {
	state := newMatch(t, lineBoard(t), matchtest.Rules(t, 1, 6, 3))

	require.Equal(t, match.PhaseRolling, state.Phase)
}

func TestDecide_RejectsCommandFromParticipantWhoseTurnItIsNot(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))

	err := decideErr(t, state, &match.DrawCommand{By: matchtest.Index(t, 0)})

	require.ErrorIs(t, err, match.ErrNotYourTurn)
}

func TestDecide_RejectsCommandThatDoesNotFitThePhase(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))
	by := matchtest.Index(t, 1)
	tests := map[string]match.Command{
		"roll_while_drawing":        &match.RollCommand{By: by},
		"discard_while_drawing":     &match.DiscardCommand{By: by, HandIndex: 0},
		"choose_path_while_drawing": &match.ChoosePathCommand{By: by, To: pos(t, 1, 0)},
		"end_turn_while_drawing":    &match.EndTurnCommand{By: by},
	}
	for name, cmd := range tests {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, decideErr(t, state, cmd), match.ErrPhaseNotAllowed)
		})
	}
}

func TestDecide_RejectsEveryCommandAfterTheMatchEnded(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))
	state.Phase = match.PhaseEnded
	by := matchtest.Index(t, 1)
	tests := map[string]match.Command{
		"draw":        &match.DrawCommand{By: by},
		"discard":     &match.DiscardCommand{By: by},
		"roll":        &match.RollCommand{By: by},
		"choose_path": &match.ChoosePathCommand{By: by, To: pos(t, 1, 0)},
		"end_turn":    &match.EndTurnCommand{By: by},
		"other_turn":  &match.DrawCommand{By: matchtest.Index(t, 0)},
	}
	for name, cmd := range tests {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, decideErr(t, state, cmd), match.ErrPhaseNotAllowed)
		})
	}
}

func TestDecide_DrawMovesTheTopOfTheDrawPileToTheHand(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))

	state, events := apply(t, state, matchtest.NewRandom(), &match.DrawCommand{By: matchtest.Index(t, 1)})

	require.Len(t, events, 1)
	require.Equal(t, refs(t, 201, 201), state.Participants[1].Hand)
	require.Equal(t, refs(t, 201), state.Participants[1].DrawPile)
	require.Equal(t, match.PhaseRolling, state.Phase)
	require.Equal(t, int64(5), events[0].GetMeta().AggregateVersion)
}

func TestDecide_DrawOverTheHandLimitMovesToDiscardingAndDiscardRemovesTheChosenCard(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))
	state.Participants = append([]match.ParticipantState(nil), state.Participants...)
	state.Participants[1].Hand = refs(t, 1, 2, 3, 4, 5, 6)

	state, _ = apply(t, state, matchtest.NewRandom(), &match.DrawCommand{By: matchtest.Index(t, 1)})
	require.Equal(t, match.PhaseDiscarding, state.Phase)
	require.Len(t, state.Participants[1].Hand, 7)

	state, events := apply(t, state, matchtest.NewRandom(), &match.DiscardCommand{By: matchtest.Index(t, 1), HandIndex: 1})

	require.Equal(t, refs(t, 1, 3, 4, 5, 6, 201), state.Participants[1].Hand)
	require.Equal(t, match.PhaseRolling, state.Phase)
	require.Equal(t, refs(t, 2)[0], events[0].(*match.CardDiscardedEvent).Card)
}

func TestDecide_DiscardRejectsHandIndexOutOfRange(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))
	state.Participants = append([]match.ParticipantState(nil), state.Participants...)
	state.Participants[1].Hand = refs(t, 1, 2, 3, 4, 5, 6)
	state, _ = apply(t, state, matchtest.NewRandom(), &match.DrawCommand{By: matchtest.Index(t, 1)})

	for name, index := range map[string]int{"past_the_end": 7, "negative": -1} {
		t.Run(name, func(t *testing.T) {
			err := decideErr(t, state, &match.DiscardCommand{By: matchtest.Index(t, 1), HandIndex: index})
			require.ErrorIs(t, err, match.ErrHandIndexOutOfRange)
		})
	}
}

func TestEvolve_RejectsDrawnCardThatIsNotTheTopOfTheDrawPile(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))

	_, err := match.Evolve(state, &match.CardDrawnEvent{Card: refs(t, 999)[0]})

	require.ErrorIs(t, err, match.ErrInvalidState)
}

func TestDecide_RollGivesFacesFromDiceMinToDiceMax(t *testing.T) {
	rules := matchtest.RulesOf(t, 8000, 600, 300, 100, 30, 2, 5, 1)
	state := newMatch(t, lineBoard(t), rules)
	state, _ = apply(t, state, matchtest.NewRandom(), &match.DrawCommand{By: matchtest.Index(t, 1)})

	lowest, _ := apply(t, state, matchtest.NewRandom(), &match.RollCommand{By: matchtest.Index(t, 1)})
	highest, _ := apply(t, state, matchtest.NewHighestRandom(), &match.RollCommand{By: matchtest.Index(t, 1)})

	require.Equal(t, 2, lowest.Dice)
	require.Equal(t, 5, highest.Dice)
}

func TestDecide_RollMovesOneTileAtATimeForTheRolledFace(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))

	state, events := drawAndRoll(t, state, 1, 3)

	require.Equal(t, 3, state.Dice)
	require.Zero(t, state.RemainingSteps)
	require.Equal(t, pos(t, 3, 0), state.Participants[1].Position)
	require.Equal(t, match.PhaseEnding, state.Phase)
	require.Equal(t, 3, countOf[*match.MovedEvent](events))
	require.Equal(t, 1, countOf[*match.DiceRolledEvent](events))
}

func TestDecide_RollTurnsBackAtDeadEnd(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))

	state, _ = drawAndRoll(t, state, 1, 5)

	require.Equal(t, pos(t, 1, 0), state.Participants[1].Position)
	require.Equal(t, pos(t, 2, 0), *state.Participants[1].Previous)
}

func TestDecide_CheckpointGivesTheBonusOnlyWhileUnmarked(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))

	// 4 マス先まで行って戻り、チェックポイントを 2 回通る（2 回目は印があるので足さない）
	state, events := drawAndRoll(t, state, 1, 5)

	require.Equal(t, 1, countOf[*match.CheckpointPassedEvent](events))
	require.Equal(t, 700, state.Participants[1].Magic)
	require.Equal(t, []board.Position{pos(t, 1, 0)}, state.Participants[1].PassedCheckpoints)
}

func TestDecide_ForkStopsToChooseThePathAndRejectsAPlaceThatCannotBeChosen(t *testing.T) {
	state := newMatch(t, forkBoard(t), defaultRules(t))

	state, _ = drawAndRoll(t, state, 1, 3)

	require.Equal(t, match.PhaseChoosingPath, state.Phase)
	require.Equal(t, 2, state.RemainingSteps)
	require.Equal(t, []board.Position{pos(t, 2, 0), pos(t, 1, 1)}, state.PathChoices())
	err := decideErr(t, state, &match.ChoosePathCommand{By: matchtest.Index(t, 1), To: pos(t, 0, 0)})
	require.ErrorIs(t, err, match.ErrPathNotAllowed)
}

func TestDecide_ChoosingThePathContinuesTheRemainingSteps(t *testing.T) {
	state := newMatch(t, forkBoard(t), defaultRules(t))
	state, _ = drawAndRoll(t, state, 1, 3)

	state, events := choose(t, state, 1, 1, 1)

	// 行き止まりの (1,1) に入って、残りの 1 歩で折り返す
	require.Equal(t, pos(t, 1, 0), state.Participants[1].Position)
	require.Equal(t, pos(t, 1, 1), *state.Participants[1].Previous)
	require.Equal(t, match.PhaseEnding, state.Phase)
	require.Equal(t, 1, countOf[*match.CheckpointPassedEvent](events))
}

func TestDecide_FirstStepOffTheBaseOffersEveryConnectedTile(t *testing.T) {
	state := newMatch(t, ringBoard(t), defaultRules(t))

	state, _ = drawAndRoll(t, state, 1, 2)

	require.Equal(t, match.PhaseChoosingPath, state.Phase)
	require.Equal(t, []board.Position{pos(t, 1, 0), pos(t, 0, 1)}, state.PathChoices())
}

func TestDecide_LapBonusIsGivenOnTheBaseOnlyWithEveryCheckpointMarkedAndClearsTheMarks(t *testing.T) {
	state := newMatch(t, ringBoard(t), defaultRules(t))
	state, _ = drawAndRoll(t, state, 1, 6)

	state, events := choose(t, state, 1, 1, 0)

	require.Equal(t, pos(t, 0, 0), state.Participants[1].Position)
	require.Equal(t, 1, countOf[*match.LapCompletedEvent](events))
	require.Equal(t, 600+100+100+300, state.Participants[1].Magic)
	require.Empty(t, state.Participants[1].PassedCheckpoints)
}

func TestDecide_NoLapBonusOnTheBaseWhileACheckpointIsUnmarked(t *testing.T) {
	state := newMatch(t, forkBoard(t), defaultRules(t))
	state, _ = drawAndRoll(t, state, 1, 4)
	state, _ = choose(t, state, 1, 2, 0)

	state, events := choose(t, state, 1, 0, 0)

	require.Equal(t, pos(t, 0, 0), state.Participants[1].Position)
	require.Zero(t, countOf[*match.LapCompletedEvent](events))
	require.Equal(t, 700, state.Participants[1].Magic)
	require.Equal(t, match.PhaseEnding, state.Phase)
}

func TestDecide_EnteringTheBaseAtTheTargetMagicWinsEvenWithStepsLeftAndCheckpointsUnmarked(t *testing.T) {
	rules := matchtest.RulesOf(t, 700, 600, 0, 100, 30, 1, 6, 1)
	state := newMatch(t, forkBoard(t), rules)
	state, _ = drawAndRoll(t, state, 1, 5)
	state, _ = choose(t, state, 1, 2, 0)

	state, events := choose(t, state, 1, 0, 0)

	require.Equal(t, match.PhaseEnded, state.Phase)
	require.Equal(t, 1, state.RemainingSteps)
	require.Equal(t, &match.Result{Winners: indexes(t, 1), Reason: match.EndReasonTargetMagic}, state.Result)
	require.IsType(t, &match.MatchEndedEvent{}, events[len(events)-1])
}

func TestDecide_LapBonusCountsTowardTheTargetMagicWhenEnteringTheBase(t *testing.T) {
	rules := matchtest.RulesOf(t, 1000, 600, 300, 100, 30, 1, 6, 1)
	b := lineBoard(t)
	b.Tiles = b.Tiles[:2]
	b.Connections = b.Connections[:1]
	state := newMatch(t, b, rules)

	state, _ = drawAndRoll(t, state, 1, 4)

	require.Equal(t, match.PhaseEnded, state.Phase)
	require.Equal(t, 1000, state.Participants[1].Magic)
	require.Equal(t, 2, state.RemainingSteps)
}

func TestDecide_EndTurnPassesTheTurnAndKeepsTheRoundUntilTheLastParticipant(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))
	state, _ = drawAndRoll(t, state, 1, 1)

	state, events := endTurn(t, state, 1)

	require.IsType(t, &match.TurnEndedEvent{}, events[0])
	current, _ := state.CurrentParticipant()
	require.Equal(t, matchtest.Index(t, 0), current)
	require.Equal(t, 1, state.Round)
	require.Equal(t, match.PhaseDrawing, state.Phase)
	require.Zero(t, state.Dice)
	require.Zero(t, state.RemainingSteps)
	require.Equal(t, pos(t, 1, 0), state.Participants[1].Position)
	require.NotNil(t, state.Participants[1].Previous)
}

func TestDecide_EndTurnAfterTheLastParticipantStartsTheNextRound(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))
	state, _ = drawAndRoll(t, state, 1, 1)
	state, _ = endTurn(t, state, 1)
	state, _ = drawAndRoll(t, state, 0, 1)

	state, _ = endTurn(t, state, 0)

	current, _ := state.CurrentParticipant()
	require.Equal(t, matchtest.Index(t, 1), current)
	require.Equal(t, 2, state.Round)
}

func TestDecide_EndTurnAtTheRoundLimitEndsTheMatchWithoutAdvancingTheRound(t *testing.T) {
	rules := matchtest.RulesOf(t, 8000, 600, 300, 100, 1, 1, 6, 1)
	state := newMatch(t, lineBoard(t), rules)
	state, _ = drawAndRoll(t, state, 1, 1)
	state, _ = endTurn(t, state, 1)
	state, _ = drawAndRoll(t, state, 0, 1)

	state, events := endTurn(t, state, 0)

	require.Len(t, events, 1)
	require.IsType(t, &match.MatchEndedEvent{}, events[0])
	require.Equal(t, 1, state.Round)
	require.Equal(t, &match.Result{Winners: indexes(t, 0, 1), Reason: match.EndReasonRoundLimit}, state.Result)
	require.Equal(t, match.PhaseEnded, state.Phase)
}

func TestDecide_RoundLimitWinnerIsTheParticipantWithTheMostMagic(t *testing.T) {
	rules := matchtest.RulesOf(t, 8000, 600, 300, 100, 1, 1, 6, 1)
	state := newMatch(t, lineBoard(t), rules)
	state, _ = drawAndRoll(t, state, 1, 1)
	state, _ = endTurn(t, state, 1)
	state, _ = drawAndRoll(t, state, 0, 1)
	state.Participants = append([]match.ParticipantState(nil), state.Participants...)
	state.Participants[0].Magic = 5000

	state, _ = endTurn(t, state, 0)

	require.Equal(t, indexes(t, 0), state.Result.Winners)
}
