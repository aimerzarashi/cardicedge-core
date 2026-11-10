package match_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules/gamerulestest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match/matchtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar/npcavatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck/npcdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell/spelltest"
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
		Movement: creaturetest.Movement(t, 1),
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
		&match.DiscardPileShuffledEvent{EventMeta: meta, Participant: by, DrawPile: refs(t, 102, 101)},
		&match.HandLimitExceededEvent{EventMeta: meta, Participant: by},
		&match.SpellUsedEvent{EventMeta: meta, Participant: by, HandIndex: 1, Card: cardtest.Ref(t, card.SpellEntityType, 401), Cost: cardtest.Cost(t, 40)},
		&match.MagicGainedEvent{EventMeta: meta, Participant: by, Magic: 200},
		&match.DiceCountChangedEvent{EventMeta: meta, Participant: by, Count: 2},
		&match.DiceRolledEvent{EventMeta: meta, Participant: by, Value: 4},
		&match.MovedEvent{EventMeta: meta, Participant: by, To: boardtest.Position(t, 1, 0)},
		&match.CheckpointPassedEvent{EventMeta: meta, Participant: by, Checkpoint: boardtest.Position(t, 1, 0), Magic: 100},
		&match.LapCompletedEvent{EventMeta: meta, Participant: by, Magic: 300},
		&match.TurnEndedEvent{EventMeta: meta, Participant: by},
		&match.MatchEndedEvent{EventMeta: meta, Winners: indexes(t, 0, 1), Ranks: []int{1, 1}, Reason: match.EndReasonRoundLimit},
		&match.RewardDrawnEvent{EventMeta: meta, Participant: by, Pack: match.DefaultPackKey, Cards: refs(t, 301, 302)},
		&match.RewardsCompletedEvent{EventMeta: meta},
		&match.TollPaidEvent{EventMeta: meta, Participant: by, Owner: matchtest.Index(t, 0), Position: boardtest.Position(t, 1, 0), Magic: 60},
		&match.TerritoryRelinquishedEvent{EventMeta: meta, Participant: by, Position: boardtest.Position(t, 1, 0), Creature: matchtest.DeployedCreatureID(t, 1), Magic: 80},
		&match.MagicDepletedEvent{EventMeta: meta, Participant: by, Magic: 600},
		&match.CreatureDeployedEvent{EventMeta: meta, Participant: by, HandIndex: 2, Card: cardtest.CreatureRef(t, 101), Creature: matchtest.DeployedCreatureID(t, 1), Position: boardtest.Position(t, 1, 0), Cost: cardtest.Cost(t, 10)},
		&match.TerritoryInvadedEvent{EventMeta: meta, Participant: by, HandIndex: 2, Card: cardtest.CreatureRef(t, 101), Position: boardtest.Position(t, 1, 0), Cost: cardtest.Cost(t, 10)},
		&match.ItemUsedEvent{EventMeta: meta, Participant: by, HandIndex: 0, Card: cardtest.Ref(t, card.ItemEntityType, 301), Cost: cardtest.Cost(t, 20)},
		&match.CreatureStruckEvent{EventMeta: meta, Striker: match.BattleSideAttacker, Damage: creaturetest.Attack(t, 10), RemainingHitPoints: creaturetest.HitPoints(t, 5)},
		&match.TerritoryConqueredEvent{EventMeta: meta, Participant: by, Position: boardtest.Position(t, 1, 0), DefeatedCreature: matchtest.DeployedCreatureID(t, 1), Creature: matchtest.DeployedCreatureID(t, 2), TerritoryHitPoints: creaturetest.HitPoints(t, 20)},
		&match.InvasionRepelledEvent{EventMeta: meta, Participant: by, Position: boardtest.Position(t, 1, 0), Card: cardtest.CreatureRef(t, 101), ReturnedToHand: true, TerritoryHitPoints: creaturetest.HitPoints(t, 15)},
		&match.ItemSkippedEvent{EventMeta: meta, Participant: by},
		&match.TerritoryLeveledUpEvent{EventMeta: meta, Participant: by, Position: boardtest.Position(t, 1, 0), Level: 3, Magic: 300},
		&match.CreatureMovedEvent{EventMeta: meta, Participant: by, From: boardtest.Position(t, 1, 0), To: boardtest.Position(t, 2, 0), Creature: matchtest.DeployedCreatureID(t, 3)},
		&match.TerritoryInvadedByMoveEvent{EventMeta: meta, Participant: by, From: boardtest.Position(t, 1, 0), Position: boardtest.Position(t, 2, 0), Creature: matchtest.DeployedCreatureID(t, 3)},
		&match.LandElementChangedEvent{EventMeta: meta, Participant: by, Position: boardtest.Position(t, 1, 0), Element: board.ElementWater, Magic: 200},
		&match.CreatureExchangedEvent{EventMeta: meta, Participant: by, Position: boardtest.Position(t, 1, 0), HandIndex: 0, Card: cardtest.CreatureRef(t, 101), Creature: matchtest.DeployedCreatureID(t, 3), Cost: cardtest.Cost(t, 10), ReturnedCard: cardtest.CreatureRef(t, 102)},
		&match.BattleStartedEvent{EventMeta: meta, Attacker: battleStats(t, 30, 40), Defender: battleStats(t, 20, 35)},
		&match.BattleStatModifiedEvent{EventMeta: meta, Side: match.BattleSideAttacker, Source: match.NewAdjacentTerritoriesSource([]board.Position{boardtest.Position(t, 0, 0), boardtest.Position(t, 2, 0)}), AttackDelta: 20, Attack: creaturetest.Attack(t, 50), HitPoints: creaturetest.HitPoints(t, 40)},
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

func TestDecide_DrawDoesNotCheckTheHandLimit(t *testing.T) {
	state := withHand(t, newMatch(t, lineBoard(t), defaultRules(t)), 1, 1, 2, 3, 4, 5, 6)

	state, _ = apply(t, state, matchtest.NewRandom(), &match.DrawCommand{By: matchtest.Index(t, 1)})

	require.Len(t, state.Participants[1].Hand, 7)
	require.Equal(t, match.PhaseRolling, state.Phase)
}

func TestDecide_DrawFromAnEmptyDrawPileShufflesTheDiscardPileIntoIt(t *testing.T) {
	state := withPiles(t, newMatch(t, lineBoard(t), defaultRules(t)), 1, nil, refs(t, 5, 6, 7))

	state, events := apply(t, state, matchtest.NewRandom(), &match.DrawCommand{By: matchtest.Index(t, 1)})

	require.Len(t, events, 2)
	shuffled := eventOf[*match.DiscardPileShuffledEvent](t, events)
	require.ElementsMatch(t, refs(t, 5, 6, 7), shuffled.DrawPile)
	require.Equal(t, shuffled.DrawPile[0], eventOf[*match.CardDrawnEvent](t, events).Card)
	require.Empty(t, state.Participants[1].DiscardPile)
	require.Equal(t, shuffled.DrawPile[1:], state.Participants[1].DrawPile)
	require.Equal(t, match.PhaseRolling, state.Phase)
}

func TestDecide_TurnStartsWithDrawingWhileTheDiscardPileHasCards(t *testing.T) {
	tests := map[string]struct {
		discardPile []card.Ref
		phase       match.Phase
	}{
		"discard_pile_has_cards": {discardPile: refs(t, 5), phase: match.PhaseDrawing},
		"both_piles_are_empty":   {discardPile: nil, phase: match.PhaseRolling},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			state, _ := drawAndRoll(t, newMatch(t, lineBoard(t), defaultRules(t)), 1, 1)
			state = withPiles(t, state, 0, nil, tt.discardPile)

			state, _ = endTurn(t, state, 1)

			require.Equal(t, tt.phase, state.Phase)
		})
	}
}

func TestDecide_EndTurnOverTheHandLimitAsksToDiscardAndEndsTheTurnAfterDiscarding(t *testing.T) {
	state, _ := drawAndRoll(t, newMatch(t, lineBoard(t), defaultRules(t)), 1, 1)
	state = withHand(t, state, 1, 1, 2, 3, 4, 5, 6, 7)

	state, events := endTurn(t, state, 1)
	require.Len(t, events, 1)
	require.IsType(t, &match.HandLimitExceededEvent{}, events[0])
	require.Equal(t, match.PhaseDiscarding, state.Phase)

	state, events = apply(t, state, matchtest.NewRandom(), &match.DiscardCommand{By: matchtest.Index(t, 1), HandIndex: 1})

	require.Len(t, events, 2)
	require.Equal(t, refs(t, 2)[0], events[0].(*match.CardDiscardedEvent).Card)
	require.IsType(t, &match.TurnEndedEvent{}, events[1])
	require.Equal(t, refs(t, 1, 3, 4, 5, 6, 7), state.Participants[1].Hand)
	require.Equal(t, refs(t, 2), state.Participants[1].DiscardPile)
	current, _ := state.CurrentParticipant()
	require.Equal(t, matchtest.Index(t, 0), current)
}

func TestDecide_DiscardKeepsAskingWhileTheHandIsOverTheLimit(t *testing.T) {
	state, _ := drawAndRoll(t, newMatch(t, lineBoard(t), defaultRules(t)), 1, 1)
	state, _ = endTurn(t, withHand(t, state, 1, 1, 2, 3, 4, 5, 6, 7, 8), 1)

	state, events := apply(t, state, matchtest.NewRandom(), &match.DiscardCommand{By: matchtest.Index(t, 1), HandIndex: 0})

	require.Len(t, events, 1)
	require.Equal(t, match.PhaseDiscarding, state.Phase)
}

func TestDecide_EndTurnAtTheRoundLimitEndsTheMatchWithoutDiscarding(t *testing.T) {
	rules := matchtest.RulesOf(t, 8000, 600, 300, 100, 1, 1, 6, 1)
	state := newMatch(t, lineBoard(t), rules)
	state, _ = drawAndRoll(t, state, 1, 1)
	state, _ = endTurn(t, state, 1)
	state, _ = drawAndRoll(t, state, 0, 1)

	_, events := endTurn(t, withHand(t, state, 0, 1, 2, 3, 4, 5, 6, 7), 0)

	require.Len(t, events, 1)
	require.IsType(t, &match.MatchEndedEvent{}, events[0])
}

func TestDecide_DiscardRejectsHandIndexOutOfRange(t *testing.T) {
	state, _ := drawAndRoll(t, newMatch(t, lineBoard(t), defaultRules(t)), 1, 1)
	state, _ = endTurn(t, withHand(t, state, 1, 1, 2, 3, 4, 5, 6, 7), 1)

	for name, index := range map[string]int{"past_the_end": 7, "negative": -1} {
		t.Run(name, func(t *testing.T) {
			err := decideErr(t, state, &match.DiscardCommand{By: matchtest.Index(t, 1), HandIndex: index})
			require.ErrorIs(t, err, match.ErrHandIndexOutOfRange)
		})
	}
}

func TestEvolve_RejectsShuffledDrawPileThatIsNotTheDiscardPile(t *testing.T) {
	state := withPiles(t, newMatch(t, lineBoard(t), defaultRules(t)), 1, nil, refs(t, 5, 6))

	_, err := match.Evolve(state, &match.DiscardPileShuffledEvent{Participant: matchtest.Index(t, 1), DrawPile: refs(t, 5, 7)})

	require.ErrorIs(t, err, match.ErrInvalidState)
}

// 参加者の手札を差し替える
func withHand(t *testing.T, state match.Match, participant int, ids ...int64) match.Match {
	t.Helper()
	state.Participants = slices.Clone(state.Participants)
	state.Participants[participant].Hand = refs(t, ids...)
	return state
}

// 参加者の山札と捨て札を差し替える
func withPiles(t *testing.T, state match.Match, participant int, drawPile, discardPile []card.Ref) match.Match {
	t.Helper()
	state.Participants = slices.Clone(state.Participants)
	state.Participants[participant].DrawPile = drawPile
	state.Participants[participant].DiscardPile = discardPile
	return state
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
	require.Equal(t, match.PhaseInvading, state.Phase)
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

// 拠点 (0,0) から東へ、火 (1,0)・火 (2,0)・火 (3,0)・水 (4,0)・複 (5,0)・無 (6,0)・無 (7,0) の一本道
func territoryBoard(t *testing.T) match.Board {
	t.Helper()
	tiles := []board.Tile{boardtest.Tile(t, 0, 0, "base", "")}
	var connections []board.Connection
	for i, element := range []string{"fire", "fire", "fire", "water", "multi", "neutral", "neutral"} {
		tiles = append(tiles, boardtest.Tile(t, i+1, 0, "land", element))
		connections = append(connections, boardtest.Connection(t, i, 0, i+1, 0))
	}
	return match.Board{ID: boardtest.ID(t, 5), Size: boardtest.Size(t, 8, 1), Tiles: tiles, Connections: connections}
}

// NPC（参加者 1）が先の番の対戦で、領地を直接置く（ここで確かめたいのは配置の後の決まりなので、配置の手順を毎回踏まない）
func withTerritory(t *testing.T, state match.Match, owner, x, level int) match.Match {
	t.Helper()
	id := matchtest.DeployedCreatureID(t, state.LastDeployedCreatureID.Int()+1)
	spec := creatureSpec(t, 201)
	state.DeployedCreatures = append(slices.Clone(state.DeployedCreatures), match.DeployedCreature{
		ID: id, Card: cardtest.CreatureRef(t, 201), Owner: matchtest.Index(t, owner), Base: *spec, HitPoints: spec.HitPoints,
	})
	state.Territories = append(slices.Clone(state.Territories), match.Territory{Position: pos(t, x, 0), Creature: id})
	if level > 1 {
		state.LandLevels = append(slices.Clone(state.LandLevels), match.LandLevel{Position: pos(t, x, 0), Level: level})
	}
	state.LastDeployedCreatureID = id
	return state
}

func withMagic(state match.Match, participant, magic int) match.Match {
	state.Participants = slices.Clone(state.Participants)
	state.Participants[participant].Magic = magic
	return state
}

func valueAt(t *testing.T, state match.Match, x int) int {
	t.Helper()
	territory, ok := state.TerritoryAt(pos(t, x, 0))
	require.True(t, ok)
	return state.TerritoryValue(territory)
}

func TestMatch_TerritoryValueMultipliesTheBaseTheLevelAndTheChain(t *testing.T) {
	type owned struct{ owner, x, level int }
	tests := []struct {
		name        string
		territories []owned
		x           int
		want        int
	}{
		{name: "level_one_without_chain", territories: []owned{{0, 1, 1}}, x: 1, want: 100},
		{name: "level_multiplier", territories: []owned{{0, 1, 3}}, x: 1, want: 400},
		{name: "two_of_the_same_element", territories: []owned{{0, 1, 1}, {0, 2, 1}}, x: 1, want: 150},
		{name: "three_of_the_same_element", territories: []owned{{0, 1, 1}, {0, 2, 1}, {0, 3, 1}}, x: 2, want: 200},
		{name: "level_and_chain_together", territories: []owned{{0, 1, 2}, {0, 2, 1}}, x: 1, want: 300},
		{name: "other_element_does_not_chain", territories: []owned{{0, 1, 1}, {0, 4, 1}}, x: 1, want: 100},
		{name: "other_owner_does_not_chain", territories: []owned{{0, 1, 1}, {1, 2, 1}}, x: 1, want: 100},
		{name: "neutral_chains", territories: []owned{{0, 6, 1}, {0, 7, 1}}, x: 6, want: 150},
		{name: "multi_does_not_chain", territories: []owned{{0, 5, 1}, {0, 1, 1}}, x: 5, want: 100},
		{name: "level_above_the_list_uses_the_last", territories: []owned{{0, 1, 9}}, x: 1, want: 1600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := newMatch(t, territoryBoard(t), defaultRules(t))
			for _, territory := range tt.territories {
				state = withTerritory(t, state, territory.owner, territory.x, territory.level)
			}
			require.Equal(t, tt.want, valueAt(t, state, tt.x))
		})
	}
}

func TestMatch_ChainAboveTheListUsesTheLastMultiplier(t *testing.T) {
	state := newMatch(t, territoryBoard(t), defaultRules(t))
	state.Rules.ChainMultipliers = gamerulestest.ChainMultipliers(t, 100, 150)
	for _, x := range []int{1, 2, 3} {
		state = withTerritory(t, state, 0, x, 1)
	}

	require.Equal(t, 150, valueAt(t, state, 1))
}

func TestMatch_TotalMagicAddsTheValueOfOwnTerritories(t *testing.T) {
	state := newMatch(t, territoryBoard(t), defaultRules(t))
	state = withTerritory(t, state, 0, 1, 1)
	state = withTerritory(t, state, 0, 2, 1)

	require.Equal(t, []int{600 + 150 + 150, 600}, state.TotalMagics())
}

// 通行料を払う
func payToll(t *testing.T, state match.Match, by int) (match.Match, []match.Event) {
	t.Helper()
	return apply(t, state, matchtest.NewRandom(), &match.PayTollCommand{By: matchtest.Index(t, by)})
}

func TestDecide_StoppingOnAnotherTerritoryOffersToPayOrInvadeWithoutPayingYet(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1)

	state, events := drawAndRoll(t, state, 1, 1)

	require.Zero(t, countOf[*match.TollPaidEvent](events))
	require.Equal(t, match.PhaseInvading, state.Phase)
	require.Equal(t, 600, state.Participants[1].Magic)
	require.Equal(t, []int{0, 1}, state.InvadeChoices())
}

// 通行料しか選べなくても、払う操作を待つ（2026-11-09 オーナー）
func TestDecide_StoppingOnAnotherTerritoryWaitsEvenWithoutACreatureThatCanBePaid(t *testing.T) {
	state := withMagic(withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1), 1, 5)

	state, _ = drawAndRoll(t, state, 1, 1)

	require.Equal(t, match.PhaseInvading, state.Phase)
	require.Empty(t, state.InvadeChoices())
}

func TestDecide_PayTollPaysAndEndsTheTurn(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1)
	state, _ = drawAndRoll(t, state, 1, 1)

	state, events := payToll(t, state, 1)

	require.Equal(t, 1, countOf[*match.TollPaidEvent](events))
	require.Equal(t, 600-60, state.Participants[1].Magic)
	require.Equal(t, 600+60, state.Participants[0].Magic)
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

func TestDecide_TollRoundsDown(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1)
	state.Rules.BaseLandValue = 101

	state, _ = drawAndRoll(t, state, 1, 1)
	state, _ = payToll(t, state, 1)

	require.Equal(t, 600-60, state.Participants[1].Magic)
}

func TestDecide_PassingThroughATerritoryPaysNothing(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1)

	state, events := drawAndRoll(t, state, 1, 2)

	require.Zero(t, countOf[*match.TollPaidEvent](events))
	require.Equal(t, match.PhaseDeploying, state.Phase)
}

func TestDecide_StoppingOnOwnTerritoryPaysNothing(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 1, 1, 1)

	state, events := drawAndRoll(t, state, 1, 1)

	require.Zero(t, countOf[*match.TollPaidEvent](events))
	require.Equal(t, 600, state.Participants[1].Magic)
	require.Equal(t, match.PhaseTerritoryCommanding, state.Phase)
}

// NPC が 10 の魔力で通行料 60 を払い、-50 になる。手放せる領地は (2,0) と (4,0)
func indebtedMatch(t *testing.T) match.Match {
	t.Helper()
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1)
	state = withTerritory(t, state, 1, 2, 1)
	state = withTerritory(t, state, 1, 4, 1)
	state = withMagic(state, 1, 10)
	state, _ = drawAndRoll(t, state, 1, 1)
	state, _ = payToll(t, state, 1)
	return state
}

func TestDecide_UnpayableTollIsPaidInFullAndAsksToRelinquish(t *testing.T) {
	state := indebtedMatch(t)

	require.Equal(t, -50, state.Participants[1].Magic)
	require.Equal(t, 600+60, state.Participants[0].Magic)
	require.Equal(t, match.PhaseRelinquishing, state.Phase)
	require.Equal(t, []board.Position{pos(t, 2, 0), pos(t, 4, 0)}, state.RelinquishChoices())
}

func TestDecide_RelinquishingKeepsTheLandLevel(t *testing.T) {
	state := indebtedMatch(t)
	state.LandLevels = []match.LandLevel{{Position: pos(t, 2, 0), Level: 3}}

	state, _ = apply(t, state, matchtest.NewRandom(), &match.RelinquishTerritoryCommand{By: matchtest.Index(t, 1), Position: pos(t, 2, 0)})

	_, occupied := state.TerritoryAt(pos(t, 2, 0))
	require.False(t, occupied)
	require.Equal(t, 3, state.LandLevel(pos(t, 2, 0)))
}

func TestDecide_DeployingOnALeveledVacantLandInheritsTheLevel(t *testing.T) {
	state := territoryBoardMatch(t)
	state.LandLevels = []match.LandLevel{{Position: pos(t, 1, 0), Level: 3}}
	state = rolledTo(t, state, 1)
	require.Equal(t, match.PhaseDeploying, state.Phase)

	state, _ = apply(t, state, matchtest.NewRandom(), &match.DeployCreatureCommand{By: matchtest.Index(t, 1), HandIndex: 0})

	require.Equal(t, 3, state.LandLevel(pos(t, 1, 0)))
	require.Equal(t, 400, valueAt(t, state, 1))
}

func TestMatch_LandLevelIsOneUntilRaised(t *testing.T) {
	require.Equal(t, 1, territoryBoardMatch(t).LandLevel(pos(t, 1, 0)))
}

func TestDecide_RelinquishingRemovesTheCreatureAndFreesTheLand(t *testing.T) {
	state := indebtedMatch(t)

	state, events := apply(t, state, matchtest.NewRandom(), &match.RelinquishTerritoryCommand{By: matchtest.Index(t, 1), Position: pos(t, 2, 0)})

	require.Equal(t, 1, countOf[*match.TerritoryRelinquishedEvent](events))
	require.Equal(t, -50+80, state.Participants[1].Magic)
	_, occupied := state.TerritoryAt(pos(t, 2, 0))
	require.False(t, occupied)
	_, deployed := state.DeployedCreature(matchtest.DeployedCreatureID(t, 2))
	require.False(t, deployed)
	require.Equal(t, []card.Ref{cardtest.CreatureRef(t, 201)}, state.Participants[1].DiscardPile)
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

func TestDecide_RelinquishingContinuesWhileMagicIsNegative(t *testing.T) {
	state := withMagic(indebtedMatch(t), 1, -200)

	state, events := apply(t, state, matchtest.NewRandom(), &match.RelinquishTerritoryCommand{By: matchtest.Index(t, 1), Position: pos(t, 2, 0)})

	require.Zero(t, countOf[*match.MagicDepletedEvent](events))
	require.Equal(t, -200+80, state.Participants[1].Magic)
	require.Equal(t, match.PhaseRelinquishing, state.Phase)
	require.Equal(t, []board.Position{pos(t, 4, 0)}, state.RelinquishChoices())
}

func TestDecide_RelinquishPriceIncludesTheChain(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1)
	state = withTerritory(t, state, 1, 2, 1)
	state = withTerritory(t, state, 1, 3, 1)
	state = withMagic(state, 1, 10)
	state, _ = drawAndRoll(t, state, 1, 1)
	state, _ = payToll(t, state, 1)

	state, _ = apply(t, state, matchtest.NewRandom(), &match.RelinquishTerritoryCommand{By: matchtest.Index(t, 1), Position: pos(t, 2, 0)})

	require.Equal(t, -50+120, state.Participants[1].Magic)
}

func TestDecide_RejectsRelinquishingWhatIsNotOwned(t *testing.T) {
	state := indebtedMatch(t)

	require.ErrorIs(t, decideErr(t, state, &match.RelinquishTerritoryCommand{By: matchtest.Index(t, 1), Position: pos(t, 1, 0)}), match.ErrTerritoryNotRelinquishable)
}

func TestDecide_RejectsRelinquishingOutsideTheRelinquishingPhase(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 1, 2, 1)

	require.ErrorIs(t, decideErr(t, state, &match.RelinquishTerritoryCommand{By: matchtest.Index(t, 1), Position: pos(t, 2, 0)}), match.ErrPhaseNotAllowed)
}

func TestDecide_MagicDepletedRestartsFromTheBaseAndEndsTheTurn(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1)
	state = withMagic(state, 1, 10)
	state.Participants[1].PassedCheckpoints = []board.Position{pos(t, 7, 0)}
	handBefore := len(state.Participants[1].Hand)
	state, _ = drawAndRoll(t, state, 1, 1)

	state, events := payToll(t, state, 1)

	require.Equal(t, 1, countOf[*match.MagicDepletedEvent](events))
	require.Zero(t, countOf[*match.LapCompletedEvent](events))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
	require.Equal(t, 600, state.Participants[1].Magic)
	require.Equal(t, pos(t, 0, 0), state.Participants[1].Position)
	require.Nil(t, state.Participants[1].Previous)
	require.Empty(t, state.Participants[1].PassedCheckpoints)
	// 引いた 1 枚の分だけ増えて、そのまま残る
	require.Len(t, state.Participants[1].Hand, handBefore+1)
	turn, _ := state.CurrentParticipant()
	require.Equal(t, matchtest.Index(t, 0), turn)
}

func TestDecide_RelinquishingTheLastTerritoryWhileNegativeDepletesTheMagic(t *testing.T) {
	state := withMagic(indebtedMatch(t), 1, -500)
	state, _ = apply(t, state, matchtest.NewRandom(), &match.RelinquishTerritoryCommand{By: matchtest.Index(t, 1), Position: pos(t, 2, 0)})

	state, events := apply(t, state, matchtest.NewRandom(), &match.RelinquishTerritoryCommand{By: matchtest.Index(t, 1), Position: pos(t, 4, 0)})

	require.Equal(t, 1, countOf[*match.MagicDepletedEvent](events))
	require.Equal(t, 600, state.Participants[1].Magic)
	// 残っているのは相手の領地だけ
	require.Len(t, state.Territories, 1)
}

func TestMatch_LowestValueRelinquishChoiceTakesTheCheapestThenTheEarliest(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1)
	state = withTerritory(t, state, 1, 2, 2)
	state = withTerritory(t, state, 1, 4, 1)
	state = withTerritory(t, state, 1, 5, 1)
	state = withMagic(state, 1, 10)
	state, _ = drawAndRoll(t, state, 1, 1)
	state, _ = payToll(t, state, 1)

	position, ok := state.LowestValueRelinquishChoice()

	require.True(t, ok)
	require.Equal(t, pos(t, 4, 0), position)
}

func TestDecide_TargetMagicCountsTheValueOfTerritories(t *testing.T) {
	rules := matchtest.RulesOf(t, 700, 600, 0, 0, 30, 1, 6, 1)
	b := territoryBoard(t)
	b.Connections = append(b.Connections, boardtest.Connection(t, 0, 0, 7, 0))
	state := withTerritory(t, newMatch(t, b, rules), 1, 1, 1)
	state = withMagic(state, 1, 600)
	state.Participants[1].Position = pos(t, 1, 0)
	previous := pos(t, 2, 0)
	state.Participants[1].Previous = &previous

	state, _ = drawAndRoll(t, state, 1, 1)

	require.Equal(t, match.PhaseEnded, state.Phase)
	require.Equal(t, []match.ParticipantIndex{matchtest.Index(t, 1)}, state.Result.Winners)
	require.Equal(t, 600, state.Participants[1].Magic)
}

func TestDecide_RoundLimitWinnerIsDecidedByTotalMagic(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), matchtest.RulesOf(t, 8000, 600, 0, 0, 1, 1, 6, 1)), 0, 1, 1)
	state = withMagic(state, 0, 550)
	state = withMagic(state, 1, 600)
	state, _ = drawAndRoll(t, state, 1, 2)
	state, _ = endTurn(t, state, 1)
	state, _ = drawAndRoll(t, state, 0, 2)
	state, _ = endTurn(t, state, 0)

	require.Equal(t, match.PhaseEnded, state.Phase)
	require.Equal(t, []match.ParticipantIndex{matchtest.Index(t, 0)}, state.Result.Winners)
	require.Equal(t, []int{1, 2}, state.Result.Ranks)
}

func TestEvolve_RejectsRelinquishedEventThatDoesNotMatchTheState(t *testing.T) {
	state := indebtedMatch(t)
	meta := match.EventMeta{EntityID: state.ID, AggregateVersion: state.Version + 1, OccurredAt: startedAt}
	valid := match.TerritoryRelinquishedEvent{EventMeta: meta, Participant: matchtest.Index(t, 1), Position: pos(t, 2, 0), Creature: matchtest.DeployedCreatureID(t, 2), Magic: 80}

	tests := []struct {
		name   string
		mutate func(e *match.TerritoryRelinquishedEvent)
	}{
		{name: "other_position", mutate: func(e *match.TerritoryRelinquishedEvent) { e.Position = pos(t, 3, 0) }},
		{name: "other_creature", mutate: func(e *match.TerritoryRelinquishedEvent) { e.Creature = matchtest.DeployedCreatureID(t, 3) }},
		{name: "territory_of_another_owner", mutate: func(e *match.TerritoryRelinquishedEvent) {
			e.Position = pos(t, 1, 0)
			e.Creature = matchtest.DeployedCreatureID(t, 1)
		}},
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

// NPC（参加者 1）が、プレイヤー（参加者 0）の領地 (1,0) に止まった対戦。NPC の手札はカード 201（コスト 10・攻撃 5・体力 20）が 2 枚で、
// 領地のクリーチャーも同じ性能
func invadingMatch(t *testing.T) match.Match {
	t.Helper()
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1)
	state, _ = drawAndRoll(t, state, 1, 1)
	require.Equal(t, match.PhaseInvading, state.Phase)
	return state
}

// 手札の最後にアイテムを足す（デッキの写しにも足す。性能は写しから読むため）
func withItemInHand(t *testing.T, state match.Match, participant int, id int64, cost int) match.Match {
	t.Helper()
	ref := cardtest.Ref(t, card.ItemEntityType, id)
	spec := match.CardSpec{Cost: cardtest.Cost(t, cost), Rarity: card.RarityNormal}
	state.Participants = slices.Clone(state.Participants)
	target := state.Participants[participant]
	target.Hand = append(slices.Clone(target.Hand), ref)
	target.Participant.Cards = append(slices.Clone(target.Participant.Cards), match.DeckCard{
		Card: ref, Quantity: cardtest.DeckQuantity(t, 1), Spec: spec,
	})
	state.Participants[participant] = target
	return state
}

// NPC が手札の最初で侵略を確定した、アイテムを選ぶ前の対戦
func attacking(t *testing.T, state match.Match) match.Match {
	t.Helper()
	state, _ = apply(t, state, matchtest.NewRandom(), &match.InvadeCommand{By: matchtest.Index(t, 1), HandIndex: 0})
	return state
}

// NPC が手札の最初で侵略し、アイテムを出さなかった後の対戦
func invade(t *testing.T, state match.Match) match.Match {
	t.Helper()
	state, _ = apply(t, attacking(t, state), matchtest.NewRandom(), &match.AttackCommand{By: matchtest.Index(t, 1)})
	return state
}

func defend(t *testing.T, state match.Match) (match.Match, []match.Event) {
	t.Helper()
	return apply(t, state, matchtest.NewRandom(), &match.DefendCommand{By: matchtest.Index(t, 0)})
}

// 侵略しているクリーチャーと領地のクリーチャーの性能を変える
func withBattlers(t *testing.T, state match.Match, attackerAttack, attackerHitPoints, defenderAttack, defenderHitPoints int) match.Match {
	t.Helper()
	require.NotNil(t, state.Invasion)
	invasion := *state.Invasion
	invasion.Base.Attack = creaturetest.Attack(t, attackerAttack)
	invasion.HitPoints = creaturetest.HitPoints(t, attackerHitPoints)
	state.Invasion = &invasion
	state.DeployedCreatures = slices.Clone(state.DeployedCreatures)
	state.DeployedCreatures[0].Base.Attack = creaturetest.Attack(t, defenderAttack)
	state.DeployedCreatures[0].HitPoints = creaturetest.HitPoints(t, defenderHitPoints)
	return state
}

func eventOf[T match.Event](t *testing.T, events []match.Event) T {
	t.Helper()
	for _, event := range events {
		if found, ok := event.(T); ok {
			return found
		}
	}
	var missing T
	require.FailNow(t, "event not found")
	return missing
}

func TestDecide_InvadePaysTheCostAndWaitsForTheOwnerOfTheTerritory(t *testing.T) {
	state := invadingMatch(t)
	handBefore := len(state.Participants[1].Hand)

	state = invade(t, state)

	require.Equal(t, match.PhaseDefending, state.Phase)
	require.Equal(t, 600-10, state.Participants[1].Magic)
	require.Len(t, state.Participants[1].Hand, handBefore-1)
	require.Equal(t, cardtest.CreatureRef(t, 201), state.Invasion.Card)
	require.Equal(t, creaturetest.HitPoints(t, 20), state.Invasion.HitPoints)
	awaiting, ok := state.AwaitingParticipant()
	require.True(t, ok)
	require.Equal(t, matchtest.Index(t, 0), awaiting)
	turn, _ := state.CurrentParticipant()
	require.Equal(t, matchtest.Index(t, 1), turn)
}

func TestDecide_InvadeIsSettledAtOnceAndAsksTheAttackerForAnItem(t *testing.T) {
	state := invadingMatch(t)
	handBefore := len(state.Participants[1].Hand)

	state = attacking(t, state)

	require.Equal(t, match.PhaseAttacking, state.Phase)
	require.Equal(t, 600-10, state.Participants[1].Magic)
	require.Len(t, state.Participants[1].Hand, handBefore-1)
	require.NotNil(t, state.Invasion)
	awaiting, _ := state.AwaitingParticipant()
	require.Equal(t, matchtest.Index(t, 1), awaiting)
}

func TestDecide_AttackWithAnItemPaysItAndAsksTheOwner(t *testing.T) {
	state := withItemInHand(t, attacking(t, invadingMatch(t)), 1, 301, 20)
	itemIndex := len(state.Participants[1].Hand) - 1

	state, events := apply(t, state, matchtest.NewRandom(), &match.AttackCommand{By: matchtest.Index(t, 1), ItemHandIndex: &itemIndex})

	used := eventOf[*match.ItemUsedEvent](t, events)
	require.Equal(t, itemIndex, used.HandIndex)
	require.Equal(t, matchtest.Index(t, 1), used.Participant)
	require.Equal(t, 600-10-20, state.Participants[1].Magic)
	require.NotContains(t, state.Participants[1].Hand, used.Card)
	require.Equal(t, match.PhaseDefending, state.Phase)
	awaiting, _ := state.AwaitingParticipant()
	require.Equal(t, matchtest.Index(t, 0), awaiting)
}

func TestDecide_AttackWithoutAnItemLeavesTheFactAndAsksTheOwner(t *testing.T) {
	state, events := apply(t, attacking(t, invadingMatch(t)), matchtest.NewRandom(), &match.AttackCommand{By: matchtest.Index(t, 1)})

	require.Equal(t, matchtest.Index(t, 1), eventOf[*match.ItemSkippedEvent](t, events).Participant)
	require.Equal(t, match.PhaseDefending, state.Phase)
}

func TestDecide_InvadeRejectsCardsThatCannotBeUsed(t *testing.T) {
	state := withItemInHand(t, invadingMatch(t), 1, 301, 20)
	itemIndex := len(state.Participants[1].Hand) - 1
	cases := []struct {
		name  string
		magic int
		hand  int
		err   error
	}{
		{name: "hand_out_of_range", magic: 600, hand: 99, err: match.ErrHandIndexOutOfRange},
		{name: "item_as_the_creature", magic: 600, hand: itemIndex, err: match.ErrCreatureNotInvadable},
		{name: "creature_cost_not_payable", magic: 5, hand: 0, err: match.ErrCreatureNotInvadable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := decideErr(t, withMagic(state, 1, tc.magic), &match.InvadeCommand{By: matchtest.Index(t, 1), HandIndex: tc.hand})

			require.ErrorIs(t, err, tc.err)
		})
	}
}

// クリーチャーのコストを払った後の魔力で、出せるアイテムが決まる
func TestDecide_AttackRejectsItemsThatCannotBeUsed(t *testing.T) {
	state := withItemInHand(t, attacking(t, invadingMatch(t)), 1, 301, 20)
	itemIndex := len(state.Participants[1].Hand) - 1
	creatureIndex := 0
	outOfHand := 99
	cases := []struct {
		name  string
		magic int
		item  *int
	}{
		{name: "creature_as_the_item", magic: 600, item: &creatureIndex},
		{name: "item_out_of_hand", magic: 600, item: &outOfHand},
		{name: "item_cost_not_payable", magic: 15, item: &itemIndex},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := decideErr(t, withMagic(state, 1, tc.magic), &match.AttackCommand{By: matchtest.Index(t, 1), ItemHandIndex: tc.item})

			require.ErrorIs(t, err, match.ErrItemNotUsable)
		})
	}
}

func TestDecide_AttackIsRejectedOutsideTheAttackingPhase(t *testing.T) {
	require.ErrorIs(t, decideErr(t, invadingMatch(t), &match.AttackCommand{By: matchtest.Index(t, 1)}), match.ErrPhaseNotAllowed)
}

func TestDecide_InvadeAndPayTollAreRejectedOutsideTheInvadingPhase(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1)
	by := matchtest.Index(t, 1)

	require.ErrorIs(t, decideErr(t, state, &match.InvadeCommand{By: by, HandIndex: 0}), match.ErrPhaseNotAllowed)
	require.ErrorIs(t, decideErr(t, state, &match.PayTollCommand{By: by}), match.ErrPhaseNotAllowed)
}

func TestDecide_DefendIsOnlyForTheOwnerOfTheInvadedTerritory(t *testing.T) {
	state := invade(t, invadingMatch(t))

	require.ErrorIs(t, decideErr(t, state, &match.DefendCommand{By: matchtest.Index(t, 1)}), match.ErrNotYourTurn)
}

func TestDecide_DefendIsRejectedOutsideTheDefendingPhase(t *testing.T) {
	state := invadingMatch(t)

	require.ErrorIs(t, decideErr(t, state, &match.DefendCommand{By: matchtest.Index(t, 1)}), match.ErrPhaseNotAllowed)
}

func TestDecide_OperationsOfTheAttackerAreRejectedWhileTheOwnerChooses(t *testing.T) {
	state := invade(t, invadingMatch(t))

	require.ErrorIs(t, decideErr(t, state, &match.EndTurnCommand{By: matchtest.Index(t, 1)}), match.ErrNotYourTurn)
}

func TestDecide_DefendWithAnItemPaysAndRemovesIt(t *testing.T) {
	state := withItemInHand(t, invade(t, invadingMatch(t)), 0, 301, 20)
	require.Equal(t, []int{len(state.Participants[0].Hand) - 1}, state.ItemChoices())
	itemIndex := len(state.Participants[0].Hand) - 1
	handBefore := len(state.Participants[0].Hand)

	state, events := apply(t, state, matchtest.NewRandom(), &match.DefendCommand{By: matchtest.Index(t, 0), ItemHandIndex: &itemIndex})

	require.Equal(t, matchtest.Index(t, 0), eventOf[*match.ItemUsedEvent](t, events).Participant)
	require.Len(t, state.Participants[0].Hand, handBefore-1)
	require.Equal(t, []card.Ref{cardtest.Ref(t, card.ItemEntityType, 301)}, state.Participants[0].DiscardPile)
	// 持ち主は通行料の 60 を受け取り、アイテムの 20 を払う
	require.Equal(t, 600+60-20, state.Participants[0].Magic)
}

func TestDecide_DefendRejectsAnItemThatCannotBeUsed(t *testing.T) {
	state := withItemInHand(t, invade(t, invadingMatch(t)), 0, 301, 20)
	creatureIndex := 0
	itemIndex := len(state.Participants[0].Hand) - 1

	require.ErrorIs(t, decideErr(t, state, &match.DefendCommand{By: matchtest.Index(t, 0), ItemHandIndex: &creatureIndex}), match.ErrItemNotUsable)
	require.ErrorIs(t, decideErr(t, withMagic(state, 0, 5), &match.DefendCommand{By: matchtest.Index(t, 0), ItemHandIndex: &itemIndex}), match.ErrItemNotUsable)
}

func TestDecide_DefeatingTheDefenderConquersTheLandKeepingItsLevelAndEndsTheTurn(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 2)
	state, _ = drawAndRoll(t, state, 1, 1)
	state = withBattlers(t, invade(t, state), 30, 20, 50, 30)
	defeated := state.Territories[0].Creature

	state, events := defend(t, state)

	// 倒した相手は反撃しない
	require.Equal(t, 1, countOf[*match.CreatureStruckEvent](events))
	struck := eventOf[*match.CreatureStruckEvent](t, events)
	require.Equal(t, match.BattleSideAttacker, struck.Striker)
	require.Equal(t, creaturetest.Attack(t, 30), struck.Damage)
	require.Zero(t, struck.RemainingHitPoints.Int())
	conquered := eventOf[*match.TerritoryConqueredEvent](t, events)
	require.Equal(t, defeated, conquered.DefeatedCreature)
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
	require.Zero(t, countOf[*match.TollPaidEvent](events))
	require.Nil(t, state.Invasion)
	require.Equal(t, []match.Territory{{Position: pos(t, 1, 0), Creature: conquered.Creature}}, state.Territories)
	require.Len(t, state.DeployedCreatures, 1)
	require.Equal(t, matchtest.Index(t, 1), state.DeployedCreatures[0].Owner)
	require.Equal(t, cardtest.CreatureRef(t, 201), state.DeployedCreatures[0].Card)
	require.Equal(t, creaturetest.HitPoints(t, 20), state.DeployedCreatures[0].HitPoints)
	require.Equal(t, state.LastDeployedCreatureID, conquered.Creature)
	require.NotEqual(t, defeated, conquered.Creature)
	require.Equal(t, []card.Ref{cardtest.CreatureRef(t, 201)}, state.Participants[0].DiscardPile)
	require.Empty(t, state.Participants[1].DiscardPile)
}

func TestDecide_FailingToDefeatTheDefenderReturnsTheSurvivorToTheHandAndPaysTheToll(t *testing.T) {
	state := invade(t, invadingMatch(t))
	handBefore := len(state.Participants[1].Hand)

	state, events := defend(t, state)

	// 攻撃 5・体力 20 どうしなので、どちらも倒れない
	require.Equal(t, 2, countOf[*match.CreatureStruckEvent](events))
	repelled := eventOf[*match.InvasionRepelledEvent](t, events)
	require.True(t, repelled.ReturnedToHand)
	require.Equal(t, cardtest.CreatureRef(t, 201), repelled.Card)
	require.Len(t, state.Participants[1].Hand, handBefore+1)
	require.Equal(t, cardtest.CreatureRef(t, 201), state.Participants[1].Hand[handBefore])
	require.Equal(t, 600-10-60, state.Participants[1].Magic)
	require.Equal(t, 600+60, state.Participants[0].Magic)
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
	require.Nil(t, state.Invasion)
	// 傷は残る
	require.Equal(t, creaturetest.HitPoints(t, 15), state.DeployedCreatures[0].HitPoints)
}

func TestDecide_AttackerDestroyedByTheCounterattackIsNotReturnedToTheHand(t *testing.T) {
	state := withBattlers(t, invade(t, invadingMatch(t)), 5, 10, 10, 20)
	handBefore := len(state.Participants[1].Hand)

	state, events := defend(t, state)

	require.False(t, eventOf[*match.InvasionRepelledEvent](t, events).ReturnedToHand)
	require.Len(t, state.Participants[1].Hand, handBefore)
	require.Equal(t, []card.Ref{cardtest.CreatureRef(t, 201)}, state.Participants[1].DiscardPile)
	require.Equal(t, 1, countOf[*match.TollPaidEvent](events))
}

func TestDecide_FailedInvasionThatCannotPayTheTollDepletesTheMagic(t *testing.T) {
	state := withMagic(invadingMatch(t), 1, 10)
	state = invade(t, state)

	state, events := defend(t, state)

	require.Equal(t, 1, countOf[*match.MagicDepletedEvent](events))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
	require.Equal(t, 600, state.Participants[1].Magic)
}

func battleStats(t *testing.T, attack, hitPoints int) match.BattleStats {
	t.Helper()
	return match.BattleStats{Attack: creaturetest.Attack(t, attack), HitPoints: creaturetest.HitPoints(t, hitPoints)}
}

// 手札の最後に、性能を持つアイテムを足し、その手札の番号を返す
func withBattleItem(t *testing.T, state match.Match, participant int, id int64, category item.Category, attackBonus, hitPointsBonus int) (match.Match, int) {
	t.Helper()
	state = withItemInHand(t, state, participant, id, 0)
	ref := cardtest.Ref(t, card.ItemEntityType, id)
	cards := slices.Clone(state.Participants[participant].Participant.Cards)
	for i := range cards {
		if cards[i].Card == ref {
			cards[i].Spec.Item = &match.ItemSpec{Category: category, AttackBonus: item.NewAttackBonus(attackBonus), HitPointsBonus: item.NewHitPointsBonus(hitPointsBonus)}
		}
	}
	state.Participants[participant].Participant.Cards = cards
	return state, len(state.Participants[participant].Hand) - 1
}

func attackWithItem(t *testing.T, state match.Match, itemHandIndex int) match.Match {
	t.Helper()
	state, _ = apply(t, state, matchtest.NewRandom(), &match.AttackCommand{By: matchtest.Index(t, 1), ItemHandIndex: &itemHandIndex})
	return state
}

func defendWithItem(t *testing.T, state match.Match, itemHandIndex int) (match.Match, []match.Event) {
	t.Helper()
	return apply(t, state, matchtest.NewRandom(), &match.DefendCommand{By: matchtest.Index(t, 0), ItemHandIndex: &itemHandIndex})
}

func withDefenderElements(t *testing.T, state match.Match, elements ...string) match.Match {
	t.Helper()
	state.DeployedCreatures = slices.Clone(state.DeployedCreatures)
	state.DeployedCreatures[0].Base.Elements = creaturetest.Elements(t, elements...)
	return state
}

func statModifications(events []match.Event) []*match.BattleStatModifiedEvent {
	var modifications []*match.BattleStatModifiedEvent
	for _, event := range events {
		if modified, ok := event.(*match.BattleStatModifiedEvent); ok {
			modifications = append(modifications, modified)
		}
	}
	return modifications
}

func TestDecide_BattleReportsEachEffectInOrderBeforeTheStrikes(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 2, 2)
	state = withTerritory(t, state, 1, 1, 1)
	state = withTerritory(t, state, 1, 3, 1)
	state, _ = drawAndRoll(t, state, 1, 2)
	state, weapon := withBattleItem(t, attacking(t, state), 1, 301, item.CategoryWeapon, 25, 0)
	state = attackWithItem(t, state, weapon)
	state = withDefenderElements(t, withBattlers(t, state, 10, 40, 20, 40), "fire")
	state, armor := withBattleItem(t, state, 0, 302, item.CategoryArmor, 0, 15)

	state, events := defendWithItem(t, state, armor)

	started, ok := events[1].(*match.BattleStartedEvent)
	require.True(t, ok)
	require.Equal(t, battleStats(t, 10, 40), started.Attacker)
	require.Equal(t, battleStats(t, 20, 40), started.Defender)
	modifications := statModifications(events)
	require.Len(t, modifications, 4)

	require.Equal(t, match.BattleSideAttacker, modifications[0].Side)
	require.Equal(t, match.NewAdjacentTerritoriesSource([]board.Position{pos(t, 1, 0), pos(t, 3, 0)}), modifications[0].Source)
	require.Equal(t, 20, modifications[0].AttackDelta)
	require.Equal(t, creaturetest.Attack(t, 30), modifications[0].Attack)

	require.Equal(t, match.BattleSideAttacker, modifications[1].Side)
	require.Equal(t, match.NewItemSource(cardtest.Ref(t, card.ItemEntityType, 301)), modifications[1].Source)
	require.Equal(t, 25, modifications[1].AttackDelta)
	require.Equal(t, creaturetest.Attack(t, 55), modifications[1].Attack)

	require.Equal(t, match.BattleSideDefender, modifications[2].Side)
	require.Equal(t, match.NewTerrainSource(board.ElementFire, 2), modifications[2].Source)
	require.Equal(t, 20, modifications[2].HitPointsDelta)
	require.Equal(t, creaturetest.HitPoints(t, 60), modifications[2].HitPoints)

	require.Equal(t, match.BattleSideDefender, modifications[3].Side)
	require.Equal(t, match.NewItemSource(cardtest.Ref(t, card.ItemEntityType, 302)), modifications[3].Source)
	require.Equal(t, 15, modifications[3].HitPointsDelta)
	require.Equal(t, creaturetest.HitPoints(t, 75), modifications[3].HitPoints)

	struck := eventOf[*match.CreatureStruckEvent](t, events)
	require.Equal(t, creaturetest.Attack(t, 55), struck.Damage)
	require.Equal(t, creaturetest.HitPoints(t, 20), struck.RemainingHitPoints)
	// 55 のダメージのうち、増えた 35 を超えた 20 だけが傷になる
	repelled := eventOf[*match.InvasionRepelledEvent](t, events)
	require.Equal(t, creaturetest.HitPoints(t, 20), repelled.TerritoryHitPoints)
	require.Equal(t, creaturetest.HitPoints(t, 20), state.DeployedCreatures[0].HitPoints)
	require.Nil(t, state.Battle)
}

func TestDecide_AdjacentTerritoriesCountOnlyConnectedTerritoriesOfTheAttacker(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 2, 1)
	state = withTerritory(t, state, 0, 1, 1)
	state = withTerritory(t, state, 1, 3, 1)
	state = withTerritory(t, state, 1, 5, 1)
	state.Board.Pairs = []board.Pair{boardtest.Pair(t, 2, 0, 5, 0)}
	state, _ = drawAndRoll(t, state, 1, 2)
	state = invade(t, state)

	_, events := defend(t, state)

	modifications := statModifications(events)
	require.Len(t, modifications, 1)
	require.Equal(t, match.NewAdjacentTerritoriesSource([]board.Position{pos(t, 3, 0)}), modifications[0].Source)
	require.Equal(t, 10, modifications[0].AttackDelta)
}

func TestDecide_TerrainAddsHitPointsWhenTheElementsMatch(t *testing.T) {
	cases := []struct {
		name     string
		x        int
		elements []string
		want     int
	}{
		{name: "same_element", x: 1, elements: []string{"fire"}, want: 10},
		{name: "one_of_the_elements", x: 1, elements: []string{"fire", "water"}, want: 10},
		{name: "other_element", x: 1, elements: []string{"water"}, want: 0},
		{name: "multi_land_with_an_element", x: 5, elements: []string{"water"}, want: 10},
		{name: "multi_land_with_neutral", x: 5, elements: []string{"neutral"}, want: 0},
		{name: "neutral_land_with_neutral", x: 6, elements: []string{"neutral"}, want: 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, tc.x, 1)
			state, _ = drawAndRoll(t, state, 1, tc.x)
			state = withDefenderElements(t, invade(t, state), tc.elements...)

			_, events := defend(t, state)

			modifications := statModifications(events)
			if tc.want == 0 {
				require.Empty(t, modifications)
				return
			}
			require.Len(t, modifications, 1)
			require.Equal(t, tc.want, modifications[0].HitPointsDelta)
		})
	}
}

func TestDecide_HitPointsAddedByEffectsAreReducedFirstAndTheRestDisappears(t *testing.T) {
	cases := []struct {
		name   string
		attack int
		want   int
	}{
		{name: "damage_within_the_bonus", attack: 10, want: 40},
		{name: "damage_beyond_the_bonus", attack: 30, want: 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 2)
			state, _ = drawAndRoll(t, state, 1, 1)
			state = withDefenderElements(t, withBattlers(t, invade(t, state), tc.attack, 20, 5, 40), "fire")

			state, _ = defend(t, state)

			require.Equal(t, creaturetest.HitPoints(t, tc.want), state.DeployedCreatures[0].HitPoints)
		})
	}
}

func TestDecide_HitPointsLoweredByAnItemComeBackAfterTheBattle(t *testing.T) {
	state := withBattlers(t, invade(t, invadingMatch(t)), 5, 20, 5, 40)
	state, cursed := withBattleItem(t, state, 0, 302, item.CategoryArmor, 0, -10)

	state, events := defendWithItem(t, state, cursed)

	require.Equal(t, creaturetest.HitPoints(t, 25), eventOf[*match.CreatureStruckEvent](t, events).RemainingHitPoints)
	require.Equal(t, creaturetest.HitPoints(t, 35), state.DeployedCreatures[0].HitPoints)
}

func TestDecide_AttackerDestroyedByAnEffectEndsTheBattleAtThatPoint(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1)
	state, _ = drawAndRoll(t, state, 1, 1)
	state, cursed := withBattleItem(t, attacking(t, state), 1, 301, item.CategoryArmor, 0, -99)
	state = attackWithItem(t, state, cursed)
	state = withDefenderElements(t, withBattlers(t, state, 50, 20, 5, 30), "fire")
	handBefore := len(state.Participants[1].Hand)

	state, events := defend(t, state)

	modifications := statModifications(events)
	require.Len(t, modifications, 1)
	require.Equal(t, -20, modifications[0].HitPointsDelta)
	require.Zero(t, modifications[0].HitPoints.Int())
	require.Zero(t, countOf[*match.CreatureStruckEvent](events))
	repelled := eventOf[*match.InvasionRepelledEvent](t, events)
	require.False(t, repelled.ReturnedToHand)
	require.Equal(t, creaturetest.HitPoints(t, 30), repelled.TerritoryHitPoints)
	require.Len(t, state.Participants[1].Hand, handBefore)
	require.Equal(t, 1, countOf[*match.TollPaidEvent](events))
}

func TestDecide_DefenderDestroyedByAnEffectIsConqueredWithoutStrikes(t *testing.T) {
	state := withBattlers(t, invade(t, invadingMatch(t)), 5, 20, 50, 30)
	state, cursed := withBattleItem(t, state, 0, 302, item.CategoryArmor, 0, -99)

	state, events := defendWithItem(t, state, cursed)

	require.Zero(t, countOf[*match.CreatureStruckEvent](events))
	conquered := eventOf[*match.TerritoryConqueredEvent](t, events)
	require.Equal(t, creaturetest.HitPoints(t, 20), conquered.TerritoryHitPoints)
	require.Equal(t, matchtest.Index(t, 1), state.DeployedCreatures[0].Owner)
	require.Equal(t, creaturetest.HitPoints(t, 20), state.DeployedCreatures[0].HitPoints)
}

func TestDecide_AttackReducedToZeroDoesNotDestroyTheCreature(t *testing.T) {
	state := withBattlers(t, invade(t, invadingMatch(t)), 5, 20, 5, 30)
	state, cursed := withBattleItem(t, state, 0, 302, item.CategoryWeapon, -50, 0)

	state, events := defendWithItem(t, state, cursed)

	modifications := statModifications(events)
	require.Len(t, modifications, 1)
	require.Equal(t, -5, modifications[0].AttackDelta)
	require.Zero(t, modifications[0].Attack.Int())
	require.Equal(t, 2, countOf[*match.CreatureStruckEvent](events))
	require.True(t, eventOf[*match.InvasionRepelledEvent](t, events).ReturnedToHand)
	// AT が 0 の反撃は傷を付けない
	require.Equal(t, creaturetest.HitPoints(t, 25), state.DeployedCreatures[0].HitPoints)
}

func TestDecide_ItemsWithoutAStatChangeEmitNoModification(t *testing.T) {
	cases := []struct {
		name     string
		category item.Category
		attack   int
		hit      int
	}{
		{name: "scroll_with_bonuses", category: item.CategoryScroll, attack: 30, hit: 30},
		{name: "weapon_with_zero_bonuses", category: item.CategoryWeapon, attack: 0, hit: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, used := withBattleItem(t, invade(t, invadingMatch(t)), 0, 302, tc.category, tc.attack, tc.hit)

			_, events := defendWithItem(t, state, used)

			require.Empty(t, statModifications(events))
		})
	}
}

// NPC（参加者 1）が先の番で、face の目だけ進んで止まった対戦
func rolledTo(t *testing.T, state match.Match, face int) match.Match {
	t.Helper()
	state, _ = drawAndRoll(t, state, 1, face)
	return state
}

func territoryBoardMatch(t *testing.T) match.Match {
	t.Helper()
	return newMatch(t, territoryBoard(t), defaultRules(t))
}

func levelUp(t *testing.T, state match.Match, x, level int) (match.Match, []match.Event) {
	t.Helper()
	return apply(t, state, matchtest.NewRandom(), &match.LevelUpTerritoryCommand{By: matchtest.Index(t, 1), Position: pos(t, x, 0), Level: level})
}

func TestDecide_PassingOwnTerritoryOffersLevelUpsWithinTheMagic(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 2)

	require.Equal(t, match.PhaseDeploying, state.Phase)
	// 手持ちの 600 では 3 まで（4 は 700 かかる）
	require.Equal(t, []match.LevelUpChoice{
		{Position: pos(t, 1, 0), Level: 2, Magic: 100},
		{Position: pos(t, 1, 0), Level: 3, Magic: 300},
	}, state.LevelUpChoices())
}

func TestDecide_TerritoryWhereTheTurnStartedIsNotOffered(t *testing.T) {
	state := withTerritory(t, territoryBoardMatch(t), 1, 1, 1)
	state.Participants = slices.Clone(state.Participants)
	base := pos(t, 0, 0)
	state.Participants[1].Position = pos(t, 1, 0)
	state.Participants[1].Previous = &base

	state = rolledTo(t, state, 1)

	require.Equal(t, match.PhaseDeploying, state.Phase)
	require.Empty(t, state.LevelUpChoices())
}

func TestDecide_TerritoryWhereTheTurnStoppedIsOffered(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 1)

	require.Equal(t, match.PhaseTerritoryCommanding, state.Phase)
	require.Equal(t, pos(t, 1, 0), state.LevelUpChoices()[0].Position)
}

func TestDecide_TerritoriesOfOthersAreNotOffered(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 0, 1, 1), 2)

	require.Equal(t, match.PhaseDeploying, state.Phase)
}

func TestDecide_PassedTerritoriesAreForgottenWhenTheTurnEnds(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 2)

	state, _ = endTurn(t, state, 1)

	require.Empty(t, state.PassedPositions)
}

func TestDecide_TerritoryCommandsAreOfferedAlongsideTheActionOfTheStoppedTile(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*testing.T, match.Match) match.Match
		face  int
		want  match.Phase
	}{
		{name: "vacant_land", setup: func(_ *testing.T, state match.Match) match.Match { return state }, face: 2, want: match.PhaseDeploying},
		{name: "territory_of_another", setup: func(t *testing.T, state match.Match) match.Match { return withTerritory(t, state, 0, 2, 1) }, face: 2, want: match.PhaseInvading},
		{name: "own_territory", setup: func(_ *testing.T, state match.Match) match.Match { return state }, face: 1, want: match.PhaseTerritoryCommanding},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := rolledTo(t, tc.setup(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1)), tc.face)

			require.Equal(t, tc.want, state.Phase)
			require.NotEmpty(t, state.LevelUpChoices())
			require.NotEmpty(t, state.ExchangeChoices())
		})
	}
}

func TestDecide_DoingNothingWhereThereIsNoActionEndsTheTurn(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 1)

	_, events := endTurn(t, state, 1)

	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

func TestDecide_TollCannotBeSkippedByEndingTheTurnOnTheTerritoryOfAnother(t *testing.T) {
	state := rolledTo(t, withTerritory(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 0, 2, 1), 2)

	require.ErrorIs(t, decideErr(t, state, &match.EndTurnCommand{By: matchtest.Index(t, 1)}), match.ErrPhaseNotAllowed)
}

func TestDecide_LevelingUpPaysTheMagicRaisesTheValueAndEndsTheTurnOnVacantLand(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 2)

	state, events := levelUp(t, state, 1, 3)

	leveled := eventOf[*match.TerritoryLeveledUpEvent](t, events)
	require.Equal(t, 3, leveled.Level)
	require.Equal(t, 300, leveled.Magic)
	require.Equal(t, 600-300, state.Participants[1].Magic)
	require.Equal(t, 400, valueAt(t, state, 1))
	require.Zero(t, countOf[*match.CreatureDeployedEvent](events))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

func TestDecide_LevelingUpOnTheTerritoryOfAnotherOnlyPaysTheToll(t *testing.T) {
	state := withTerritory(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 0, 2, 1)
	state = rolledTo(t, state, 2)

	state, events := levelUp(t, state, 1, 2)

	require.Equal(t, 1, countOf[*match.TollPaidEvent](events))
	require.Zero(t, countOf[*match.TerritoryInvadedEvent](events))
	require.Equal(t, 600-100-60, state.Participants[1].Magic)
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
	require.Empty(t, state.InvadeChoices())
}

func TestDecide_LevelUpRejectsChoicesThatAreNotOffered(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 2)
	cases := []struct {
		name  string
		x     int
		level int
	}{
		{name: "not_passed", x: 3, level: 2},
		{name: "same_level", x: 1, level: 1},
		{name: "not_payable", x: 1, level: 4},
		{name: "above_the_highest_level", x: 1, level: 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := decideErr(t, state, &match.LevelUpTerritoryCommand{By: matchtest.Index(t, 1), Position: pos(t, tc.x, 0), Level: tc.level})

			require.ErrorIs(t, err, match.ErrTerritoryNotLevelUpable)
		})
	}
}

// 通った領地にいるクリーチャーを、手札に無いカードにして傷を付ける（手札に戻るカードを見分けるため）
func withWoundedRare(t *testing.T, state match.Match) match.Match {
	t.Helper()
	state.DeployedCreatures = slices.Clone(state.DeployedCreatures)
	state.DeployedCreatures[0].Card = cardtest.CreatureRef(t, 999)
	state.DeployedCreatures[0].HitPoints = creaturetest.HitPoints(t, 1)
	return state
}

func exchange(t *testing.T, state match.Match, x, handIndex int) (match.Match, []match.Event) {
	t.Helper()
	return apply(t, state, matchtest.NewRandom(), &match.ExchangeCreatureCommand{By: matchtest.Index(t, 1), Position: pos(t, x, 0), HandIndex: handIndex})
}

func TestMatch_ExchangeChoicesAreCreaturesPayableWithTheMagic(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 2)
	state = withItemInHand(t, state, 1, 301, 20)

	choices := state.ExchangeChoices()

	require.Len(t, choices, len(state.Participants[1].Hand)-1)
	require.Equal(t, match.ExchangeChoice{Position: pos(t, 1, 0), HandIndex: 0, Magic: 10}, choices[0])
	require.Empty(t, withMagic(state, 1, 5).ExchangeChoices())
}

func TestDecide_ExchangeReplacesTheCreatureAndReturnsTheOldCardToTheHand(t *testing.T) {
	state := withTerritory(t, territoryBoardMatch(t), 1, 1, 3)
	state = withWoundedRare(t, state)
	state = rolledTo(t, state, 2)
	handBefore := len(state.Participants[1].Hand)
	replaced := state.DeployedCreatures[0].ID

	state, events := exchange(t, state, 1, 0)

	exchanged := eventOf[*match.CreatureExchangedEvent](t, events)
	require.Equal(t, cardtest.CreatureRef(t, 201), exchanged.Card)
	require.Equal(t, cardtest.CreatureRef(t, 999), exchanged.ReturnedCard)
	require.NotEqual(t, replaced, exchanged.Creature)
	require.Equal(t, 600-10, state.Participants[1].Magic)
	require.Len(t, state.Participants[1].Hand, handBefore)
	require.Equal(t, cardtest.CreatureRef(t, 999), state.Participants[1].Hand[handBefore-1])
	require.Len(t, state.DeployedCreatures, 1)
	require.Equal(t, exchanged.Creature, state.DeployedCreatures[0].ID)
	require.Equal(t, creaturetest.HitPoints(t, 20), state.DeployedCreatures[0].HitPoints)
	require.Equal(t, matchtest.Index(t, 1), state.DeployedCreatures[0].Owner)
	require.Equal(t, []match.Territory{{Position: pos(t, 1, 0), Creature: exchanged.Creature}}, state.Territories)
	require.Equal(t, state.LastDeployedCreatureID, exchanged.Creature)
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

func TestDecide_ExchangingOnTheTerritoryOfAnotherOnlyPaysTheToll(t *testing.T) {
	state := withTerritory(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 0, 2, 1)
	state = rolledTo(t, state, 2)

	_, events := exchange(t, state, 1, 0)

	require.Equal(t, 1, countOf[*match.TollPaidEvent](events))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

func TestDecide_ExchangeRejectsChoicesThatAreNotOffered(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 2)
	state = withItemInHand(t, state, 1, 301, 20)
	itemIndex := len(state.Participants[1].Hand) - 1
	cases := []struct {
		name string
		x    int
		hand int
	}{
		{name: "not_passed", x: 3, hand: 0},
		{name: "item_as_the_creature", x: 1, hand: itemIndex},
		{name: "hand_out_of_range", x: 1, hand: 99},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := decideErr(t, state, &match.ExchangeCreatureCommand{By: matchtest.Index(t, 1), Position: pos(t, tc.x, 0), HandIndex: tc.hand})

			require.ErrorIs(t, err, match.ErrCreatureNotExchangeable)
		})
	}
}

func changeElement(t *testing.T, state match.Match, x int, element board.Element) (match.Match, []match.Event) {
	t.Helper()
	return apply(t, state, matchtest.NewRandom(), &match.ChangeLandElementCommand{By: matchtest.Index(t, 1), Position: pos(t, x, 0), Element: element})
}

func TestMatch_ElementChangeChoicesAreOtherElementsWithinTheMagic(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 2)

	require.Equal(t, []match.ElementChangeChoice{
		{Position: pos(t, 1, 0), Element: board.ElementEarth, Magic: 200},
		{Position: pos(t, 1, 0), Element: board.ElementWater, Magic: 200},
		{Position: pos(t, 1, 0), Element: board.ElementWind, Magic: 200},
	}, state.ElementChangeChoices())
	require.Empty(t, withMagic(state, 1, 199).ElementChangeChoices())
}

func TestMatch_ChangingFromMultiLandIsCheaperAndOffersAllFourElements(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 5, 1), 5)

	choices := state.ElementChangeChoices()

	require.Len(t, choices, 4)
	for _, choice := range choices {
		require.Equal(t, 100, choice.Magic)
	}
}

func TestDecide_ChangingTheElementRecountsTheChainAndEndsTheTurn(t *testing.T) {
	state := withTerritory(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 2), 1, 2, 1)
	state = rolledTo(t, state, 3)
	require.Equal(t, 150, valueAt(t, state, 2))

	state, events := changeElement(t, state, 1, board.ElementWater)

	changed := eventOf[*match.LandElementChangedEvent](t, events)
	require.Equal(t, board.ElementWater, changed.Element)
	require.Equal(t, 200, changed.Magic)
	require.Equal(t, 600-200, state.Participants[1].Magic)
	require.Equal(t, 100, valueAt(t, state, 2))
	// レベルとクリーチャーはそのまま
	require.Equal(t, 2, state.LandLevel(pos(t, 1, 0)))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

func TestDecide_ChangingTheElementOnTheTerritoryOfAnotherOnlyPaysTheToll(t *testing.T) {
	state := withTerritory(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 0, 2, 1)
	state = rolledTo(t, state, 2)

	_, events := changeElement(t, state, 1, board.ElementWater)

	require.Equal(t, 1, countOf[*match.TollPaidEvent](events))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

func TestDecide_ElementChangeRejectsChoicesThatAreNotOffered(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 2)
	cases := []struct {
		name    string
		x       int
		element board.Element
	}{
		{name: "same_element", x: 1, element: board.ElementFire},
		{name: "to_multi", x: 1, element: board.ElementMulti},
		{name: "to_neutral", x: 1, element: board.ElementNeutral},
		{name: "not_passed", x: 3, element: board.ElementWater},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := decideErr(t, state, &match.ChangeLandElementCommand{By: matchtest.Index(t, 1), Position: pos(t, tc.x, 0), Element: tc.element})

			require.ErrorIs(t, err, match.ErrLandElementNotChangeable)
		})
	}
}

// その位置の領地のクリーチャーの性能を変える
func withCreatureAt(t *testing.T, state match.Match, x, attack, hitPoints, movement int) match.Match {
	t.Helper()
	territory, ok := state.TerritoryAt(pos(t, x, 0))
	require.True(t, ok)
	state.DeployedCreatures = slices.Clone(state.DeployedCreatures)
	for i := range state.DeployedCreatures {
		if state.DeployedCreatures[i].ID == territory.Creature {
			state.DeployedCreatures[i].Base.Attack = creaturetest.Attack(t, attack)
			state.DeployedCreatures[i].Base.Movement = creaturetest.Movement(t, movement)
			state.DeployedCreatures[i].HitPoints = creaturetest.HitPoints(t, hitPoints)
		}
	}
	return state
}

func move(t *testing.T, state match.Match, from, to int) (match.Match, []match.Event) {
	t.Helper()
	return apply(t, state, matchtest.NewRandom(), &match.MoveCreatureCommand{By: matchtest.Index(t, 1), From: pos(t, from, 0), To: pos(t, to, 0)})
}

func TestMatch_MoveChoicesWalkExactlyTheMovementWithoutTurningBack(t *testing.T) {
	tests := []struct {
		name     string
		movement int
		want     []int
	}{
		{name: "one_step", movement: 1, want: []int{2, 4}},
		{name: "two_steps", movement: 2, want: []int{1, 5}},
		// 拠点の先は行き止まりなので、3 歩では左に届かない
		{name: "three_steps", movement: 3, want: []int{6}},
		{name: "cannot_move", movement: 0, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := withCreatureAt(t, withTerritory(t, territoryBoardMatch(t), 1, 3, 1), 3, 5, 20, tt.movement)
			state = rolledTo(t, state, 3)

			var got []int
			for _, choice := range state.MoveChoices() {
				require.Equal(t, pos(t, 3, 0), choice.From)
				got = append(got, choice.To.X.Int())
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestMatch_MoveChoicesExcludeOwnTerritoriesAndTilesThatAreNotLand(t *testing.T) {
	state := withTerritory(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 1, 2, 1)
	state = rolledTo(t, state, 2)

	require.Equal(t, []match.MoveChoice{{From: pos(t, 2, 0), To: pos(t, 3, 0)}}, state.MoveChoices())
}

func TestDecide_MovingToVacantLandKeepsTheCreatureAndLeavesTheLevelsOnTheLands(t *testing.T) {
	state := withTerritory(t, territoryBoardMatch(t), 1, 1, 3)
	state.LandLevels = append(slices.Clone(state.LandLevels), match.LandLevel{Position: pos(t, 2, 0), Level: 2})
	state = withCreatureAt(t, state, 1, 5, 7, 1)
	mover := state.DeployedCreatures[0].ID
	state = rolledTo(t, state, 1)

	state, events := move(t, state, 1, 2)

	moved := eventOf[*match.CreatureMovedEvent](t, events)
	require.Equal(t, mover, moved.Creature)
	require.Equal(t, []match.Territory{{Position: pos(t, 2, 0), Creature: mover}}, state.Territories)
	require.Equal(t, creaturetest.HitPoints(t, 7), state.DeployedCreatures[0].HitPoints)
	require.Equal(t, 3, state.LandLevel(pos(t, 1, 0)))
	require.Equal(t, 2, state.LandLevel(pos(t, 2, 0)))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

// 自分（参加者 1）の領地 (1,0) から、参加者 0 の領地 (2,0) へ動いて戦闘する
func movingIntoAnother(t *testing.T, moverAttack, moverHitPoints, defenderAttack, defenderHitPoints int) (match.Match, []match.Event) {
	t.Helper()
	state := withTerritory(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 0, 2, 2)
	state = withCreatureAt(t, state, 1, moverAttack, moverHitPoints, 1)
	state = withCreatureAt(t, state, 2, defenderAttack, defenderHitPoints, 1)
	state = rolledTo(t, state, 1)
	state, _ = move(t, state, 1, 2)
	require.Equal(t, match.PhaseAttacking, state.Phase)
	require.Equal(t, pos(t, 2, 0), state.Invasion.Position)
	state, _ = apply(t, state, matchtest.NewRandom(), &match.AttackCommand{By: matchtest.Index(t, 1)})
	awaiting, _ := state.AwaitingParticipant()
	require.Equal(t, matchtest.Index(t, 0), awaiting)
	return defend(t, state)
}

func TestDecide_MovingIntoAnotherAndWinningMovesTheSameCreatureAndFreesTheOrigin(t *testing.T) {
	state, events := movingIntoAnother(t, 50, 20, 5, 20)

	conquered := eventOf[*match.TerritoryConqueredEvent](t, events)
	mover := state.DeployedCreatures[0].ID
	require.Equal(t, mover, conquered.Creature)
	require.Equal(t, []match.Territory{{Position: pos(t, 2, 0), Creature: mover}}, state.Territories)
	require.Len(t, state.DeployedCreatures, 1)
	require.Equal(t, matchtest.Index(t, 1), state.DeployedCreatures[0].Owner)
	require.Equal(t, 2, state.LandLevel(pos(t, 2, 0)))
	require.Zero(t, countOf[*match.TollPaidEvent](events))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

func TestDecide_MovingIntoAnotherAndSurvivingReturnsToTheOriginWithTheWound(t *testing.T) {
	state, events := movingIntoAnother(t, 5, 20, 5, 20)

	repelled := eventOf[*match.InvasionRepelledEvent](t, events)
	require.False(t, repelled.ReturnedToHand)
	require.Equal(t, pos(t, 1, 0), *repelled.ReturnedTo)
	require.Equal(t, creaturetest.HitPoints(t, 15), *repelled.ReturnedHitPoints)
	territory, ok := state.TerritoryAt(pos(t, 1, 0))
	require.True(t, ok)
	mover, _ := state.DeployedCreature(territory.Creature)
	require.Equal(t, creaturetest.HitPoints(t, 15), mover.HitPoints)
	require.Nil(t, state.Invasion)
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

func TestDecide_MovingIntoAnotherAndBeingDestroyedFreesTheOrigin(t *testing.T) {
	state, events := movingIntoAnother(t, 5, 5, 10, 20)

	repelled := eventOf[*match.InvasionRepelledEvent](t, events)
	require.Nil(t, repelled.ReturnedTo)
	require.Nil(t, repelled.ReturnedHitPoints)
	_, occupied := state.TerritoryAt(pos(t, 1, 0))
	require.False(t, occupied)
	require.Len(t, state.DeployedCreatures, 1)
	require.Len(t, state.Participants[1].DiscardPile, 1)
}

func TestDecide_AdjacentTerritoriesOfAMoveDoNotCountTheOrigin(t *testing.T) {
	state := withTerritory(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 0, 2, 1)
	state = withTerritory(t, state, 1, 3, 1)
	state = withCreatureAt(t, state, 1, 5, 20, 1)
	state = rolledTo(t, state, 1)
	state, _ = move(t, state, 1, 2)
	state, _ = apply(t, state, matchtest.NewRandom(), &match.AttackCommand{By: matchtest.Index(t, 1)})

	_, events := defend(t, state)

	modifications := statModifications(events)
	require.NotEmpty(t, modifications)
	require.Equal(t, match.NewAdjacentTerritoriesSource([]board.Position{pos(t, 3, 0)}), modifications[0].Source)
}

func TestDecide_MoveRejectsChoicesThatAreNotOffered(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 2)

	err := decideErr(t, state, &match.MoveCreatureCommand{By: matchtest.Index(t, 1), From: pos(t, 1, 0), To: pos(t, 3, 0)})

	require.ErrorIs(t, err, match.ErrCreatureNotMovable)
}

func TestDecide_TerritoryCommandsAreRejectedOutsideTheirPhase(t *testing.T) {
	state := territoryBoardMatch(t)
	by := matchtest.Index(t, 1)

	require.ErrorIs(t, decideErr(t, state, &match.LevelUpTerritoryCommand{By: by, Position: pos(t, 1, 0), Level: 2}), match.ErrPhaseNotAllowed)
}

func TestMatch_ItemChoicesAreForWhoeverIsBeingWaitedFor(t *testing.T) {
	attacker := withItemInHand(t, attacking(t, invadingMatch(t)), 1, 301, 20)
	require.Equal(t, []int{len(attacker.Participants[1].Hand) - 1}, attacker.ItemChoices())

	// 守る段階は持ち主の手札を見るので、攻める側のアイテムは選択肢に入らない
	defending := withItemInHand(t, invade(t, invadingMatch(t)), 1, 301, 20)
	require.Empty(t, defending.ItemChoices())
	require.Empty(t, invadingMatch(t).ItemChoices())
	require.Empty(t, newMatch(t, territoryBoard(t), defaultRules(t)).ItemChoices())
}

func TestPhase_AwaitsOperationIsFalseWhileBattling(t *testing.T) {
	require.False(t, match.PhaseBattling.AwaitsOperation())
	require.True(t, match.PhaseInvading.AwaitsOperation())
	require.True(t, match.PhaseAttacking.AwaitsOperation())
	require.True(t, match.PhaseDefending.AwaitsOperation())
}

func TestEvolve_RejectsInvasionEventsThatDoNotMatchTheState(t *testing.T) {
	invading := invadingMatch(t)
	armed := attacking(t, invadingMatch(t))
	defending := invade(t, invadingMatch(t))
	by := matchtest.Index(t, 1)
	meta := match.EventMeta{EntityID: invading.ID, AggregateVersion: invading.Version + 1, OccurredAt: startedAt}
	started, err := match.Evolve(defending, &match.BattleStartedEvent{EventMeta: meta, Attacker: battleStats(t, 5, 20), Defender: battleStats(t, 5, 20)})
	require.NoError(t, err)
	battling := started.(match.Match)
	position := pos(t, 1, 0)
	creatureRef := cardtest.CreatureRef(t, 201)
	otherRef := cardtest.CreatureRef(t, 999)
	cost := cardtest.Cost(t, 10)
	damage := creaturetest.Attack(t, 1)
	hitPoints := creaturetest.HitPoints(t, 1)
	defeated := battling.Territories[0].Creature
	next := matchtest.DeployedCreatureID(t, battling.LastDeployedCreatureID.Int()+1)
	cases := []struct {
		name  string
		state match.Match
		event match.Event
	}{
		{name: "invaded_outside_the_phase", state: defending, event: &match.TerritoryInvadedEvent{EventMeta: meta, Participant: by, Card: creatureRef, Position: position, Cost: cost}},
		{name: "invaded_elsewhere", state: invading, event: &match.TerritoryInvadedEvent{EventMeta: meta, Participant: by, Card: creatureRef, Position: pos(t, 2, 0), Cost: cost}},
		{name: "invaded_with_another_card", state: invading, event: &match.TerritoryInvadedEvent{EventMeta: meta, Participant: by, Card: otherRef, Position: position, Cost: cost}},
		{name: "item_outside_the_phase", state: invading, event: &match.ItemUsedEvent{EventMeta: meta, Participant: by, Card: creatureRef, Cost: cost}},
		{name: "item_not_in_the_hand", state: armed, event: &match.ItemUsedEvent{EventMeta: meta, Participant: by, Card: otherRef, Cost: cost}},
		{name: "item_by_who_is_not_awaited", state: defending, event: &match.ItemUsedEvent{EventMeta: meta, Participant: by, Card: creatureRef, Cost: cost}},
		{name: "skipped_outside_the_phase", state: invading, event: &match.ItemSkippedEvent{EventMeta: meta, Participant: by}},
		{name: "skipped_by_who_is_not_awaited", state: defending, event: &match.ItemSkippedEvent{EventMeta: meta, Participant: by}},
		{name: "battle_started_outside_the_phase", state: invading, event: &match.BattleStartedEvent{EventMeta: meta, Attacker: battleStats(t, 5, 20), Defender: battleStats(t, 5, 20)}},
		{name: "stat_modified_before_the_battle", state: defending, event: &match.BattleStatModifiedEvent{EventMeta: meta, Side: match.BattleSideAttacker, Attack: damage, HitPoints: hitPoints}},
		{name: "struck_before_the_battle", state: defending, event: &match.CreatureStruckEvent{EventMeta: meta, Striker: match.BattleSideAttacker, Damage: damage, RemainingHitPoints: hitPoints}},
		{name: "struck_without_invasion", state: invading, event: &match.CreatureStruckEvent{EventMeta: meta, Striker: match.BattleSideAttacker, Damage: damage, RemainingHitPoints: hitPoints}},
		{name: "conquered_before_the_battle", state: defending, event: &match.TerritoryConqueredEvent{EventMeta: meta, Participant: by, Position: position, DefeatedCreature: defeated, Creature: next}},
		{name: "conquered_another_creature", state: battling, event: &match.TerritoryConqueredEvent{EventMeta: meta, Participant: by, Position: position, DefeatedCreature: next, Creature: next}},
		{name: "conquered_with_a_used_id", state: battling, event: &match.TerritoryConqueredEvent{EventMeta: meta, Participant: by, Position: position, DefeatedCreature: defeated, Creature: defeated}},
		{name: "repelled_before_the_battle", state: defending, event: &match.InvasionRepelledEvent{EventMeta: meta, Participant: by, Position: position, Card: creatureRef, ReturnedToHand: true}},
		{name: "repelled_with_another_card", state: battling, event: &match.InvasionRepelledEvent{EventMeta: meta, Participant: by, Position: position, Card: otherRef, ReturnedToHand: true}},
		{name: "repelled_says_dead_while_alive", state: battling, event: &match.InvasionRepelledEvent{EventMeta: meta, Participant: by, Position: position, Card: creatureRef, ReturnedToHand: false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := match.Evolve(tc.state, tc.event)

			require.ErrorIs(t, err, match.ErrInvalidState)
		})
	}
}

// 手札の最後にスペルを足す（デッキの写しにも足す。性能は写しから読むため）
func withSpellInHand(t *testing.T, state match.Match, participant int, id int64, cost int, effect spell.Effect) match.Match {
	t.Helper()
	ref := cardtest.Ref(t, card.SpellEntityType, id)
	spec := match.CardSpec{Cost: cardtest.Cost(t, cost), Rarity: card.RarityNormal, Spell: &match.SpellSpec{Effect: effect}}
	state.Participants = slices.Clone(state.Participants)
	target := state.Participants[participant]
	target.Hand = append(slices.Clone(target.Hand), ref)
	target.Participant.Cards = append(slices.Clone(target.Participant.Cards), match.DeckCard{
		Card: ref, Quantity: cardtest.DeckQuantity(t, 1), Spec: spec,
	})
	state.Participants[participant] = target
	return state
}

// 参加者 1 が引いた後の、サイコロを振る段階の対戦（手札は 3 枚目にスペル 401（コスト 40・2 枚引く））
func rollingWithSpell(t *testing.T) match.Match {
	t.Helper()
	state, _ := apply(t, newMatch(t, lineBoard(t), defaultRules(t)), matchtest.NewRandom(), &match.DrawCommand{By: matchtest.Index(t, 1)})
	require.Equal(t, match.PhaseRolling, state.Phase)
	return withSpellInHand(t, state, 1, 401, 40, spelltest.DrawCards(t, 2))
}

func spellIndex(state match.Match) int {
	return len(state.Participants[1].Hand) - 1
}

func TestDecide_UseSpellPaysAndDrawsAndPutsTheSpellIntoTheDiscardPile(t *testing.T) {
	state := withPiles(t, rollingWithSpell(t), 1, refs(t, 5, 6, 7), nil)
	handBefore := len(state.Participants[1].Hand)
	require.Equal(t, []int{spellIndex(state)}, state.SpellChoices())

	state, events := apply(t, state, matchtest.NewRandom(), &match.UseSpellCommand{By: matchtest.Index(t, 1), HandIndex: spellIndex(state)})

	used := eventOf[*match.SpellUsedEvent](t, events)
	require.Equal(t, cardtest.Ref(t, card.SpellEntityType, 401), used.Card)
	require.Equal(t, cardtest.Cost(t, 40), used.Cost)
	require.Equal(t, 2, countOf[*match.CardDrawnEvent](events))
	require.Len(t, state.Participants[1].Hand, handBefore-1+2)
	require.Equal(t, refs(t, 7), state.Participants[1].DrawPile)
	require.Equal(t, []card.Ref{cardtest.Ref(t, card.SpellEntityType, 401)}, state.Participants[1].DiscardPile)
	require.Equal(t, 600-40, state.Participants[1].Magic)
	// 使った後もサイコロを振る。スペルは 1 ターンに 1 枚まで
	require.Equal(t, match.PhaseRolling, state.Phase)
	require.Empty(t, state.SpellChoices())
}

func TestDecide_UseSpellDrawsOnlyWhatIsLeftAndShufflesTheDiscardPile(t *testing.T) {
	state := withPiles(t, rollingWithSpell(t), 1, refs(t, 5), nil)

	state, events := apply(t, state, matchtest.NewRandom(), &match.UseSpellCommand{By: matchtest.Index(t, 1), HandIndex: spellIndex(state)})

	// 1 枚目は山札から、2 枚目は使ったスペルだけの捨て札を混ぜて引く
	require.Equal(t, 2, countOf[*match.CardDrawnEvent](events))
	require.Equal(t, 1, countOf[*match.DiscardPileShuffledEvent](events))
	require.Empty(t, state.Participants[1].DrawPile)
	require.Empty(t, state.Participants[1].DiscardPile)
}

func TestDecide_UseSpellStopsDrawingWhenBothPilesAreEmpty(t *testing.T) {
	state := rollingWithSpell(t)
	state = withPiles(t, state, 1, nil, nil)

	state, events := apply(t, state, matchtest.NewRandom(), &match.UseSpellCommand{By: matchtest.Index(t, 1), HandIndex: spellIndex(state)})

	// 使ったスペルが捨て札に入るので、その 1 枚だけを混ぜて引ける
	require.Equal(t, 1, countOf[*match.CardDrawnEvent](events))
	require.Equal(t, match.PhaseRolling, state.Phase)
}

func TestDecide_UseSpellRejectsWhatCannotBeUsed(t *testing.T) {
	creatureIndex := 0
	tests := map[string]struct {
		state     func(t *testing.T) match.Match
		handIndex func(state match.Match) int
		err       error
	}{
		"creature": {
			state:     rollingWithSpell,
			handIndex: func(match.Match) int { return creatureIndex },
			err:       match.ErrSpellNotUsable,
		},
		"no_effect": {
			state: func(t *testing.T) match.Match {
				return withSpellInHand(t, rollingWithSpell(t), 1, 402, 10, spell.NoEffect())
			},
			handIndex: spellIndex,
			err:       match.ErrSpellNotUsable,
		},
		"not_payable": {
			state:     func(t *testing.T) match.Match { return withMagic(rollingWithSpell(t), 1, 39) },
			handIndex: spellIndex,
			err:       match.ErrSpellNotUsable,
		},
		"already_used": {
			state: func(t *testing.T) match.Match {
				state := withSpellInHand(t, rollingWithSpell(t), 1, 403, 10, spelltest.DrawCards(t, 1))
				state, _ = apply(t, state, matchtest.NewRandom(), &match.UseSpellCommand{By: matchtest.Index(t, 1), HandIndex: spellIndex(state)})
				return state
			},
			handIndex: func(state match.Match) int { return spellIndex(state) - 1 },
			err:       match.ErrSpellNotUsable,
		},
		"out_of_range": {
			state:     rollingWithSpell,
			handIndex: func(state match.Match) int { return len(state.Participants[1].Hand) },
			err:       match.ErrHandIndexOutOfRange,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			state := tt.state(t)

			err := decideErr(t, state, &match.UseSpellCommand{By: matchtest.Index(t, 1), HandIndex: tt.handIndex(state)})

			require.ErrorIs(t, err, tt.err)
		})
	}
}

func TestDecide_UseSpellIsOnlyBeforeRolling(t *testing.T) {
	state := withSpellInHand(t, newMatch(t, lineBoard(t), defaultRules(t)), 1, 401, 40, spelltest.DrawCards(t, 1))
	require.Equal(t, match.PhaseDrawing, state.Phase)
	require.Empty(t, state.SpellChoices())

	err := decideErr(t, state, &match.UseSpellCommand{By: matchtest.Index(t, 1), HandIndex: spellIndex(state)})

	require.ErrorIs(t, err, match.ErrPhaseNotAllowed)
}

func TestDecide_SpellCanBeUsedAgainInTheNextTurn(t *testing.T) {
	state := rollingWithSpell(t)
	state, _ = apply(t, state, matchtest.NewRandom(), &match.UseSpellCommand{By: matchtest.Index(t, 1), HandIndex: spellIndex(state)})
	require.True(t, state.SpellUsed)
	state, _ = drawAndRoll(t, state, 1, 1)

	state, _ = endTurn(t, state, 1)

	require.False(t, state.SpellUsed)
}

func TestDecide_GainMagicSpellGivesMagicByTheLaps(t *testing.T) {
	tests := map[string]struct {
		laps int
		want int
	}{
		"no_laps":  {laps: 0, want: 100},
		"two_laps": {laps: 2, want: 300},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			state := withSpellInHand(t, rollingWithSpell(t), 1, 402, 0, spelltest.GainMagic(t, 100))
			state.Participants = slices.Clone(state.Participants)
			state.Participants[1].Laps = tt.laps

			state, events := apply(t, state, matchtest.NewRandom(), &match.UseSpellCommand{By: matchtest.Index(t, 1), HandIndex: spellIndex(state)})

			require.Equal(t, tt.want, eventOf[*match.MagicGainedEvent](t, events).Magic)
			require.Equal(t, 600+tt.want, state.Participants[1].Magic)
			require.Equal(t, match.PhaseRolling, state.Phase)
		})
	}
}

func TestEvolve_LapCompletedCountsTheLaps(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))

	evolved, err := match.Evolve(state, &match.LapCompletedEvent{Participant: matchtest.Index(t, 1), Magic: 300})

	require.NoError(t, err)
	require.Equal(t, 1, evolved.(match.Match).Participants[1].Laps)
}

func TestDecide_RollDiceSpellRollsTheDiceTogetherAndMovesByTheTotal(t *testing.T) {
	state := withSpellInHand(t, rollingWithSpell(t), 1, 403, 50, spelltest.RollDice(t, 2))
	state, events := apply(t, state, matchtest.NewRandom(), &match.UseSpellCommand{By: matchtest.Index(t, 1), HandIndex: spellIndex(state)})
	require.Equal(t, 2, eventOf[*match.DiceCountChangedEvent](t, events).Count)
	require.Equal(t, match.PhaseRolling, state.Phase)

	state, events = apply(t, state, matchtest.NewRollingRandom(1, 2), &match.RollCommand{By: matchtest.Index(t, 1)})

	rolled := eventOf[*match.DiceRolledEvent](t, events)
	require.Equal(t, []int{1, 2}, rolled.Dice)
	require.Equal(t, 3, rolled.Value)
	require.Equal(t, 3, state.Dice)
	state, _ = endTurn(t, state, 1)
	require.Zero(t, state.DiceCount)
}

func TestDecide_RollWithOneDieLeavesTheDiceOut(t *testing.T) {
	_, events := apply(t, rollingWithSpell(t), matchtest.NewRollingRandom(4), &match.RollCommand{By: matchtest.Index(t, 1)})

	rolled := eventOf[*match.DiceRolledEvent](t, events)
	require.Nil(t, rolled.Dice)
	require.Equal(t, 4, rolled.Value)
}

func TestEvolve_RejectsDiceThatDoNotAddUpToTheValue(t *testing.T) {
	state := rollingWithSpell(t)

	_, err := match.Evolve(state, &match.DiceRolledEvent{Participant: matchtest.Index(t, 1), Value: 5, Dice: []int{1, 2}})

	require.ErrorIs(t, err, match.ErrInvalidState)
}
