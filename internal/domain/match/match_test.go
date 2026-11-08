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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature/creaturetest"
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
	spec := match.CardSpec{Cost: cardtest.Cost(t, int(id%100)*10), Rarity: card.RarityNormal, Creature: creatureSpec(t, id)}
	return match.DeckCard{Card: cardtest.CreatureRef(t, id), Quantity: cardtest.DeckQuantity(t, quantity), Spec: spec}
}

func creatureSpec(t *testing.T, id int64) *match.CreatureSpec {
	t.Helper()
	return &match.CreatureSpec{
		Elements: creature.DefaultElements(), Attack: creaturetest.Attack(t, int(id%100)*5), HitPoints: creaturetest.HitPoints(t, int(id%100)*10+10),
	}
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
	by := matchtest.Index(t, 1)
	events := []match.Event{
		&match.CreatedEvent{EventMeta: meta, Participants: []match.Participant{playerParticipant(t), npcParticipant(t)}, Board: twoTileBoard(t), Rules: matchtest.Rules(t, 1, 6, 5)},
		&match.TurnOrderDecidedEvent{EventMeta: meta, Order: indexes(t, 1, 0)},
		&match.DecksShuffledEvent{EventMeta: meta, DrawPiles: [][]card.Ref{refs(t, 102, 101, 101), refs(t, 201, 201, 201)}},
		&match.HandsDealtEvent{EventMeta: meta, Cards: 5},
		&match.CardDrawnEvent{EventMeta: meta, Participant: by, Card: cardtest.CreatureRef(t, 101)},
		&match.CardDiscardedEvent{EventMeta: meta, Participant: by, HandIndex: 1, Card: cardtest.CreatureRef(t, 101)},
		&match.DiceRolledEvent{EventMeta: meta, Participant: by, Value: 4},
		&match.MovedEvent{EventMeta: meta, Participant: by, To: boardtest.Position(t, 1, 0)},
		&match.CheckpointPassedEvent{EventMeta: meta, Participant: by, Checkpoint: boardtest.Position(t, 1, 0), Magic: 100},
		&match.LapCompletedEvent{EventMeta: meta, Participant: by, Magic: 300},
		&match.TurnEndedEvent{EventMeta: meta, Participant: by},
		&match.MatchEndedEvent{EventMeta: meta, Winners: indexes(t, 0, 1), Ranks: []int{1, 1}, Reason: match.EndReasonRoundLimit},
		&match.RewardDrawnEvent{EventMeta: meta, Participant: by, Pack: match.DefaultPackKey, Cards: refs(t, 301, 302)},
		&match.RewardsCompletedEvent{EventMeta: meta},
		&match.CreatureDeployedEvent{EventMeta: meta, Participant: by, HandIndex: 2, Card: cardtest.CreatureRef(t, 101), Creature: matchtest.DeployedCreatureID(t, 1), Position: boardtest.Position(t, 1, 0), Cost: cardtest.Cost(t, 10)},
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

	_, err := match.Evolve(state, &match.CardDrawnEvent{Participant: matchtest.Index(t, 1), Card: refs(t, 999)[0]})

	require.ErrorIs(t, err, match.ErrInvalidState)
}

func TestEvolve_RejectsEventOfParticipantWhoseTurnItIsNot(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))
	other := matchtest.Index(t, 0)
	tests := map[string]match.Event{
		"card_drawn":        &match.CardDrawnEvent{Participant: other, Card: state.Participants[1].DrawPile[0]},
		"card_discarded":    &match.CardDiscardedEvent{Participant: other, HandIndex: 0, Card: state.Participants[1].Hand[0]},
		"dice_rolled":       &match.DiceRolledEvent{Participant: other, Value: 1},
		"moved":             &match.MovedEvent{Participant: other, To: pos(t, 1, 0)},
		"checkpoint_passed": &match.CheckpointPassedEvent{Participant: other, Checkpoint: pos(t, 1, 0), Magic: 100},
		"lap_completed":     &match.LapCompletedEvent{Participant: other, Magic: 300},
		"turn_ended":        &match.TurnEndedEvent{Participant: other},
	}
	for name, event := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := match.Evolve(state, event)
			require.ErrorIs(t, err, match.ErrInvalidState)
		})
	}
}

func TestDecide_TurnEventsCarryTheParticipantWhoseTurnItIs(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))
	by := matchtest.Index(t, 1)

	state, drawn := apply(t, state, matchtest.NewRandom(), &match.DrawCommand{By: by})
	state, rolled := apply(t, state, matchtest.NewRollingRandom(1), &match.RollCommand{By: by})
	_, ended := endTurn(t, state, 1)

	require.Equal(t, by, drawn[0].(*match.CardDrawnEvent).Participant)
	require.Equal(t, by, rolled[0].(*match.DiceRolledEvent).Participant)
	require.Equal(t, by, rolled[1].(*match.MovedEvent).Participant)
	require.Equal(t, by, ended[0].(*match.TurnEndedEvent).Participant)
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
	// 止まった (3,0) は空いている土地なので、配置を選ぶ段階に入る
	require.Equal(t, match.PhaseDeploying, state.Phase)
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
	require.Equal(t, &match.Result{Winners: indexes(t, 1), Ranks: []int{2, 1}, Reason: match.EndReasonTargetMagic}, state.Result)
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
	require.Equal(t, &match.Result{Winners: indexes(t, 0, 1), Ranks: []int{1, 1}, Reason: match.EndReasonRoundLimit}, state.Result)
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

// 参加者を n 人にした対戦を、ラウンドの上限（1）の最後の人の番の終わりまで進める。magics は終える直前の魔力
func endAtRoundLimit(t *testing.T, n int, magics []int) match.Match {
	t.Helper()
	participants := make([]match.Participant, n)
	for i := range participants {
		participants[i] = npcParticipant(t)
	}
	return endAtRoundLimitWith(t, participants, magics)
}

func endAtRoundLimitWith(t *testing.T, participants []match.Participant, magics []int) match.Match {
	t.Helper()
	cmd := startCommand(t, matchtest.RulesOf(t, 8000, 600, 300, 100, 1, 1, 6, 1), participants...)
	cmd.Board = lineBoard(t)
	state, _ := start(t, cmd)
	order := append([]match.ParticipantIndex(nil), state.TurnOrder...)
	for i, by := range order {
		state, _ = drawAndRoll(t, state, by.Int(), 1)
		if i < len(order)-1 {
			state, _ = endTurn(t, state, by.Int())
		}
	}
	state.Participants = append([]match.ParticipantState(nil), state.Participants...)
	for i, magic := range magics {
		state.Participants[i].Magic = magic
	}
	state, _ = endTurn(t, state, order[len(order)-1].Int())
	return state
}

func TestDecide_RoundLimitRanksAreTheSameForTheSameMagic(t *testing.T) {
	state := endAtRoundLimit(t, 4, []int{500, 900, 500, 100})

	require.Equal(t, []int{2, 1, 2, 4}, state.Result.Ranks)
	require.Equal(t, indexes(t, 1), state.Result.Winners)
}

func TestDecide_RoundLimitWinnersAreEveryoneRankedFirst(t *testing.T) {
	state := endAtRoundLimit(t, 3, []int{900, 100, 900})

	require.Equal(t, []int{1, 3, 1}, state.Result.Ranks)
	require.Equal(t, indexes(t, 0, 2), state.Result.Winners)
}

func TestDecide_TargetMagicWinnerIsFirstAndOthersStartFromSecondEvenWithTheSameMagic(t *testing.T) {
	rules := matchtest.RulesOf(t, 700, 600, 0, 100, 30, 1, 6, 1)
	participants := []match.Participant{npcParticipant(t), npcParticipant(t), npcParticipant(t), npcParticipant(t)}
	cmd := startCommand(t, rules, participants...)
	cmd.Board = forkBoard(t)
	state, _ := start(t, cmd)
	winner := state.TurnOrder[0].Int()
	state, _ = drawAndRoll(t, state, winner, 5)
	state, _ = choose(t, state, winner, 2, 0)
	state.Participants = append([]match.ParticipantState(nil), state.Participants...)
	for i := range state.Participants {
		if i != winner {
			state.Participants[i].Magic = 500
		}
	}
	state.Participants[(winner+1)%4].Magic = 700

	state, _ = choose(t, state, winner, 0, 0)

	want := []int{3, 3, 3, 3}
	want[winner] = 1
	want[(winner+1)%4] = 2
	require.Equal(t, want, state.Result.Ranks)
	require.Equal(t, indexes(t, winner), state.Result.Winners)
}

func secondPlayerParticipant(t *testing.T) match.Participant {
	t.Helper()
	return match.NewPlayerParticipant(playertest.ID(t, 11), avatartest.ID(t, 3), decktest.ID(t, 4), []match.DeckCard{deckCard(t, 103, 3)})
}

// プレイヤー A（0）・プレイヤー B（1）・NPC（2）が、魔力 900・500・100 で上限のラウンドを終えた対戦（順位は 1・2・3）
func endedWithTwoPlayers(t *testing.T) match.Match {
	t.Helper()
	return endAtRoundLimitWith(t, []match.Participant{playerParticipant(t), secondPlayerParticipant(t), npcParticipant(t)}, []int{900, 500, 100})
}

func chooseReward(t *testing.T, by int, candidates []card.Ref) *match.ChooseRewardPackCommand {
	return &match.ChooseRewardPackCommand{By: matchtest.Index(t, by), Pack: match.DefaultPackKey, Candidates: candidates}
}

func rewardDrawn(t *testing.T, events []match.Event) *match.RewardDrawnEvent {
	t.Helper()
	for _, event := range events {
		if drawn, ok := event.(*match.RewardDrawnEvent); ok {
			return drawn
		}
	}
	require.Fail(t, "no RewardDrawnEvent")
	return nil
}

func TestDecide_ChooseRewardPackDrawsTheCardsOfTheRankWithoutRepeats(t *testing.T) {
	state := endedWithTwoPlayers(t)
	candidates := refs(t, 1, 2, 3, 4, 5, 6, 7, 8, 1, 2)

	_, first := apply(t, state, matchtest.NewRandom(), chooseReward(t, 0, candidates))
	_, second := apply(t, state, matchtest.NewRandom(), chooseReward(t, 1, candidates))

	require.Len(t, rewardDrawn(t, first).Cards, 3)
	require.Len(t, rewardDrawn(t, second).Cards, 2)
	require.Len(t, uniqueCards(rewardDrawn(t, first).Cards), 3)
	require.Len(t, uniqueCards(rewardDrawn(t, second).Cards), 2)
	require.Equal(t, matchtest.Index(t, 0), rewardDrawn(t, first).Participant)
	require.Equal(t, match.DefaultPackKey, rewardDrawn(t, first).Pack)
}

func uniqueCards(cards []card.Ref) []card.Ref {
	seen := map[card.Ref]bool{}
	var unique []card.Ref
	for _, c := range cards {
		if !seen[c] {
			seen[c] = true
			unique = append(unique, c)
		}
	}
	return unique
}

func TestDecide_ChooseRewardPackDrawsFromTheCandidatesOnly(t *testing.T) {
	state := endedWithTwoPlayers(t)
	candidates := refs(t, 11, 12, 13, 14)

	_, events := apply(t, state, matchtest.NewRandom(), chooseReward(t, 0, candidates))

	for _, drawn := range rewardDrawn(t, events).Cards {
		require.Contains(t, candidates, drawn)
	}
}

func TestDecide_ChooseRewardPackDrawsOnlyWhatTheCandidatesHaveWhenTheyAreFewer(t *testing.T) {
	state := endedWithTwoPlayers(t)

	_, events := apply(t, state, matchtest.NewRandom(), chooseReward(t, 0, refs(t, 11, 12)))

	require.ElementsMatch(t, refs(t, 11, 12), rewardDrawn(t, events).Cards)
}

func TestDecide_ChooseRewardPackDrawsNothingWithoutCandidatesButStillCountsAsReceived(t *testing.T) {
	state := endedWithTwoPlayers(t)

	state, events := apply(t, state, matchtest.NewRandom(), chooseReward(t, 0, nil))

	require.Empty(t, rewardDrawn(t, events).Cards)
	require.NotNil(t, state.Participants[0].Reward)
}

// 順位の一覧は 3, 2, 1, 1 で、5 位以下は最後の値の 1 枚
func TestDecide_ChooseRewardPackUsesTheLastCountForLowerRanks(t *testing.T) {
	participants := make([]match.Participant, 5)
	for i := range participants {
		participants[i] = playerParticipant(t)
	}
	state := endAtRoundLimitWith(t, participants, []int{900, 800, 700, 600, 500})
	require.Equal(t, []int{1, 2, 3, 4, 5}, state.Result.Ranks)

	_, events := apply(t, state, matchtest.NewRandom(), chooseReward(t, 4, refs(t, 1, 2, 3, 4, 5)))

	require.Len(t, rewardDrawn(t, events).Cards, 1)
}

func TestDecide_ChooseRewardPackGivesTheSameRankTheSameCount(t *testing.T) {
	state := endAtRoundLimitWith(t, []match.Participant{playerParticipant(t), secondPlayerParticipant(t)}, []int{500, 500})

	_, events := apply(t, state, matchtest.NewRandom(), chooseReward(t, 1, refs(t, 1, 2, 3, 4, 5)))

	require.Len(t, rewardDrawn(t, events).Cards, 3)
}

func TestDecide_ChooseRewardPackRejectsBeforeTheMatchEnded(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))

	require.ErrorIs(t, decideErr(t, state, chooseReward(t, 0, refs(t, 1))), match.ErrPhaseNotAllowed)
}

func TestDecide_ChooseRewardPackRejectsNPC(t *testing.T) {
	state := endedWithTwoPlayers(t)

	require.ErrorIs(t, decideErr(t, state, chooseReward(t, 2, refs(t, 1))), match.ErrRewardNotForNPC)
}

func TestDecide_ChooseRewardPackRejectsSecondChoiceOfTheSamePlayer(t *testing.T) {
	state := endedWithTwoPlayers(t)
	state, _ = apply(t, state, matchtest.NewRandom(), chooseReward(t, 0, refs(t, 1, 2, 3)))

	require.ErrorIs(t, decideErr(t, state, chooseReward(t, 0, refs(t, 1, 2, 3))), match.ErrRewardAlreadyReceived)
}

func TestDecide_ChooseRewardPackRejectsPackThatCannotBeChosen(t *testing.T) {
	state := endedWithTwoPlayers(t)
	cmd := chooseReward(t, 0, refs(t, 1))
	cmd.Pack = "unknown"

	require.ErrorIs(t, decideErr(t, state, cmd), match.ErrRewardPackNotAvailable)
}

func TestDecide_ChooseRewardPackDoesNotNeedTheTurn(t *testing.T) {
	state := endedWithTwoPlayers(t)

	for _, by := range []int{0, 1} {
		_, events := apply(t, state, matchtest.NewRandom(), chooseReward(t, by, refs(t, 1, 2, 3)))
		require.Equal(t, matchtest.Index(t, by), rewardDrawn(t, events).Participant)
	}
}

func TestDecide_RewardsCompleteWhenTheLastPlayerHasChosen(t *testing.T) {
	state := endedWithTwoPlayers(t)

	state, events := apply(t, state, matchtest.NewRandom(), chooseReward(t, 0, refs(t, 1, 2, 3)))
	require.Len(t, events, 1)
	require.False(t, state.RewardsCompleted)

	state, events = apply(t, state, matchtest.NewRandom(), chooseReward(t, 1, refs(t, 1, 2, 3)))
	require.Len(t, events, 2)
	require.IsType(t, &match.RewardsCompletedEvent{}, events[1])
	require.Equal(t, events[0].GetMeta().AggregateVersion+1, events[1].GetMeta().AggregateVersion)
	require.True(t, state.RewardsCompleted)
	require.Equal(t, match.PhaseEnded, state.Phase)
}

func TestEvolve_RewardDrawnKeepsTheCardsOfThePlayer(t *testing.T) {
	state := endedWithTwoPlayers(t)

	state, events := apply(t, state, matchtest.NewRandom(), chooseReward(t, 0, refs(t, 1, 2, 3)))

	require.Equal(t, &match.Reward{Pack: match.DefaultPackKey, Cards: rewardDrawn(t, events).Cards}, state.Participants[0].Reward)
	require.Nil(t, state.Participants[1].Reward)
	require.False(t, state.CanChooseRewardPack(matchtest.Index(t, 0)))
	require.True(t, state.CanChooseRewardPack(matchtest.Index(t, 1)))
	require.False(t, state.CanChooseRewardPack(matchtest.Index(t, 2)))
}

func TestEvolve_RejectsRewardEventsThatDoNotFit(t *testing.T) {
	ended := endedWithTwoPlayers(t)
	meta := match.EventMeta{EntityID: ended.ID, AggregateVersion: ended.Version + 1, OccurredAt: startedAt}
	playing := newMatch(t, lineBoard(t), defaultRules(t))
	drawn := func(by int) match.Event {
		return &match.RewardDrawnEvent{EventMeta: meta, Participant: matchtest.Index(t, by), Pack: match.DefaultPackKey}
	}
	received, _ := apply(t, ended, matchtest.NewRandom(), chooseReward(t, 0, refs(t, 1)))
	tests := map[string]struct {
		state match.Match
		event match.Event
	}{
		"drawn_before_end":          {playing, drawn(0)},
		"drawn_for_npc":             {ended, drawn(2)},
		"drawn_twice":               {received, drawn(0)},
		"completed_before_everyone": {received, &match.RewardsCompletedEvent{EventMeta: meta}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := match.Evolve(tt.state, tt.event)
			require.ErrorIs(t, err, match.ErrInvalidState)
		})
	}
}

func TestMatch_CardSpecsReturnsSnapshotOfEveryParticipantDeck(t *testing.T) {
	meta := match.EventMeta{EntityID: matchtest.ID(t, 1), AggregateVersion: 1}
	created := &match.CreatedEvent{EventMeta: meta, Participants: []match.Participant{playerParticipant(t), npcParticipant(t)}, Board: twoTileBoard(t), Rules: matchtest.Rules(t, 1, 6, 5)}
	state, err := match.Evolve(match.InitialState(), created)
	require.NoError(t, err)

	specs := state.(match.Match).CardSpecs()

	require.Equal(t, map[card.Ref]match.CardSpec{
		cardtest.CreatureRef(t, 101): {Cost: cardtest.Cost(t, 10), Rarity: card.RarityNormal, Creature: creatureSpec(t, 101)},
		cardtest.CreatureRef(t, 102): {Cost: cardtest.Cost(t, 20), Rarity: card.RarityNormal, Creature: creatureSpec(t, 102)},
		cardtest.CreatureRef(t, 201): {Cost: cardtest.Cost(t, 10), Rarity: card.RarityNormal, Creature: creatureSpec(t, 201)},
	}, specs)
}

// 配置の段階まで進める。NPC（参加者 1）が先の番で、引いてから目 1 を振ると、空いている火属性の土地 (1,0) に止まる
func deployingMatch(t *testing.T, participants ...match.Participant) match.Match {
	t.Helper()
	cmd := startCommand(t, defaultRules(t), participants...)
	state, _ := start(t, cmd)
	state, _ = drawAndRoll(t, state, 1, 1)
	require.Equal(t, match.PhaseDeploying, state.Phase)
	return state
}

func TestDecide_StoppingOnVacantLandOffersTheCreaturesThatCanBePaid(t *testing.T) {
	// NPC のデッキ: カード 201（コスト 10）と 299（コスト 990。開始時の魔力 600 では払えない）。山札は逆順に混ざるので、
	// 配った 1 枚が 299、引いた 1 枚が 201
	npc := match.NewNPCParticipant(npcavatartest.ID(t, 7), npcdecktest.ID(t, 8), []match.DeckCard{deckCard(t, 201, 1), deckCard(t, 299, 1)})
	state := deployingMatch(t, playerParticipant(t), npc)

	require.Equal(t, refs(t, 299, 201), state.Participants[1].Hand)
	require.Equal(t, []int{1}, state.DeployChoices())
}

func TestDecide_DeployPaysTheCostMakesTheTerritoryAndEndsTheTurn(t *testing.T) {
	state := deployingMatch(t, playerParticipant(t), npcParticipant(t))
	handBefore := len(state.Participants[1].Hand)

	state, events := apply(t, state, matchtest.NewRandom(), &match.DeployCreatureCommand{By: matchtest.Index(t, 1), HandIndex: 0})

	require.Equal(t, 1, countOf[*match.CreatureDeployedEvent](events))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
	require.Equal(t, 600-10, state.Participants[1].Magic)
	require.Len(t, state.Participants[1].Hand, handBefore-1)
	require.Equal(t, []match.Territory{{Position: pos(t, 1, 0), Creature: matchtest.DeployedCreatureID(t, 1)}}, state.Territories)
	require.Equal(t, []match.DeployedCreature{{
		ID: matchtest.DeployedCreatureID(t, 1), Card: cardtest.CreatureRef(t, 201), Owner: matchtest.Index(t, 1),
		Base: *creatureSpec(t, 201), HitPoints: creatureSpec(t, 201).HitPoints,
	}}, state.DeployedCreatures)
	// 次の番（プレイヤー）に移っている
	turn, _ := state.CurrentParticipant()
	require.Equal(t, matchtest.Index(t, 0), turn)
}

func TestDecide_EndTurnWhileDeployingLeavesTheLandVacant(t *testing.T) {
	state := deployingMatch(t, playerParticipant(t), npcParticipant(t))

	state, events := endTurn(t, state, 1)

	require.Zero(t, countOf[*match.CreatureDeployedEvent](events))
	require.Empty(t, state.Territories)
	require.Equal(t, 600, state.Participants[1].Magic)
}

func TestDecide_DeployRejectsCardsThatCannotBeDeployed(t *testing.T) {
	itemSpec := match.CardSpec{Cost: cardtest.Cost(t, 10), Rarity: card.RarityNormal}
	item := match.DeckCard{Card: cardtest.Ref(t, card.ItemEntityType, 250), Quantity: cardtest.DeckQuantity(t, 1), Spec: itemSpec}
	// 山札は逆順に混ざるので、手札は先頭から 299・250（アイテム）・201
	npc := match.NewNPCParticipant(npcavatartest.ID(t, 7), npcdecktest.ID(t, 8), []match.DeckCard{deckCard(t, 201, 1), item, deckCard(t, 299, 1)})
	state := deployingMatch(t, playerParticipant(t), npc)
	hand := state.Participants[1].Hand
	require.Equal(t, cardtest.CreatureRef(t, 299), hand[0])
	require.Equal(t, cardtest.Ref(t, card.ItemEntityType, 250), hand[1])

	tests := []struct {
		name      string
		handIndex int
		want      error
	}{
		{name: "cost_exceeds_magic", handIndex: 0, want: match.ErrCreatureNotDeployable},
		{name: "not_creature", handIndex: 1, want: match.ErrCreatureNotDeployable},
		{name: "out_of_range", handIndex: len(hand), want: match.ErrHandIndexOutOfRange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := decideErr(t, state, &match.DeployCreatureCommand{By: matchtest.Index(t, 1), HandIndex: tt.handIndex})
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestDecide_StoppingOnATerritoryDoesNotOfferDeploying(t *testing.T) {
	state := deployingMatch(t, playerParticipant(t), npcParticipant(t))
	state, _ = apply(t, state, matchtest.NewRandom(), &match.DeployCreatureCommand{By: matchtest.Index(t, 1), HandIndex: 0})

	// プレイヤーも目 1 で、NPC の領地になった (1,0) に止まる
	state, _ = drawAndRoll(t, state, 0, 1)

	require.Equal(t, pos(t, 1, 0), state.Participants[0].Position)
	require.Equal(t, match.PhaseEnding, state.Phase)
	require.Empty(t, state.DeployChoices())
}

func TestDecide_DeployRejectsOutsideTheDeployingPhase(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))

	err := decideErr(t, state, &match.DeployCreatureCommand{By: matchtest.Index(t, 1), HandIndex: 0})

	require.ErrorIs(t, err, match.ErrPhaseNotAllowed)
}

func TestDecide_DeployedCreaturesGetConsecutiveIDs(t *testing.T) {
	state := newMatch(t, ringBoard(t), defaultRules(t))
	// 拠点からの最初の一歩は 2 方向に分かれる。NPC は (1,0) へ進んで目 2 で土地 (2,0)、プレイヤーは (0,1) へ進んで目 1 で土地 (0,1)
	state, _ = drawAndRoll(t, state, 1, 2)
	state, _ = choose(t, state, 1, 1, 0)
	state, _ = apply(t, state, matchtest.NewRandom(), &match.DeployCreatureCommand{By: matchtest.Index(t, 1), HandIndex: 0})
	state, _ = drawAndRoll(t, state, 0, 1)
	state, _ = choose(t, state, 0, 0, 1)
	state, _ = apply(t, state, matchtest.NewRandom(), &match.DeployCreatureCommand{By: matchtest.Index(t, 0), HandIndex: 0})

	require.Equal(t, []match.Territory{
		{Position: pos(t, 2, 0), Creature: matchtest.DeployedCreatureID(t, 1)},
		{Position: pos(t, 0, 1), Creature: matchtest.DeployedCreatureID(t, 2)},
	}, state.Territories)
	require.Equal(t, matchtest.Index(t, 0), state.DeployedCreatures[1].Owner)
	require.Equal(t, matchtest.DeployedCreatureID(t, 2), state.LastDeployedCreatureID)
}

func TestEvolve_RejectsDeployedEventThatDoesNotMatchTheState(t *testing.T) {
	state := deployingMatch(t, playerParticipant(t), npcParticipant(t))
	meta := match.EventMeta{EntityID: state.ID, AggregateVersion: state.Version + 1, OccurredAt: startedAt}
	valid := match.CreatureDeployedEvent{EventMeta: meta, Participant: matchtest.Index(t, 1), HandIndex: 0, Card: state.Participants[1].Hand[0], Creature: matchtest.DeployedCreatureID(t, 1), Position: pos(t, 1, 0), Cost: cardtest.Cost(t, 10)}

	tests := []struct {
		name   string
		mutate func(e *match.CreatureDeployedEvent)
	}{
		{name: "other_card", mutate: func(e *match.CreatureDeployedEvent) { e.Card = cardtest.CreatureRef(t, 999) }},
		{name: "other_position", mutate: func(e *match.CreatureDeployedEvent) { e.Position = pos(t, 0, 0) }},
		{name: "skipped_id", mutate: func(e *match.CreatureDeployedEvent) { e.Creature = matchtest.DeployedCreatureID(t, 2) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := valid
			tt.mutate(&event)
			_, err := match.Evolve(state, &event)
			require.ErrorIs(t, err, match.ErrInvalidState)
		})
	}
}
