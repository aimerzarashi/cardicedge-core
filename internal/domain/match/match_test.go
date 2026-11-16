package match_test

import (
	"encoding/json"
	"fmt"
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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect/effecttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/element"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules/gamerulestest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match/matchtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar/npcavatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck/npcdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
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
		ItemRestrictions: creature.DefaultItemRestrictions(), PlacementRestrictions: creature.DefaultPlacementRestrictions(), Effects: effect.DefaultEffects(),
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
		&match.DrawPilesShuffledEvent{EventMeta: meta, DrawPiles: [][]card.Ref{refs(t, 102, 101, 101), refs(t, 201, 201, 201)}},
		&match.HandsDealtEvent{EventMeta: meta, Cards: 5},
		&match.CardDrawnEvent{EventMeta: meta, Participant: by},
		&match.CardDiscardedEvent{EventMeta: meta, Participant: by, HandIndex: 1},
		&match.DiscardPileShuffledEvent{EventMeta: meta, Participant: by, DrawPile: refs(t, 102, 101)},
		&match.HandLimitExceededEvent{EventMeta: meta, Participant: by},
		&match.SpellUsedEvent{EventMeta: meta, Participant: by, HandIndex: 1, Cost: cardtest.Cost(t, 40)},
		&match.MagicGainedEvent{EventMeta: meta, Participant: by, Magic: 600},
		&match.SpellEffectStartedEvent{EventMeta: meta, Participant: by, Target: matchtest.Index(t, 0), Effect: *lasting(spelltest.FixedDice(t, 6, 1), 1)},
		&match.SpellEffectEndedEvent{EventMeta: meta, Participant: by},
		&match.CreatureDamagedEvent{EventMeta: meta, Participant: by, Position: boardtest.Position(t, 1, 0), Damage: 20, RemainingHitPoints: creaturetest.HitPoints(t, 10)},
		&match.BoardCreatureDestroyedEvent{EventMeta: meta, Participant: by},
		&match.InvadingCreatureDestroyedEvent{EventMeta: meta, Participant: by},
		&match.DefendingCreatureDestroyedEvent{EventMeta: meta, Participant: by},
		&match.DestructionCreatureEffectActivatedEvent{EventMeta: meta, Participant: by, Owner: matchtest.Index(t, 0), Card: cardtest.CreatureRef(t, 201), Effect: effect.KindReturnToHandOnDestruction},
		&match.CardReturnedToHandEvent{EventMeta: meta, Participant: by, Owner: matchtest.Index(t, 0), Card: cardtest.CreatureRef(t, 201)},
		&match.MagicReceivedEvent{EventMeta: meta, Participant: by, Owner: matchtest.Index(t, 0), Magic: 200},
		&match.DiceRolledEvent{EventMeta: meta, Participant: by, Value: 4},
		&match.PieceMovedEvent{EventMeta: meta, Participant: by, To: boardtest.Position(t, 1, 0)},
		&match.CheckpointPassedEvent{EventMeta: meta, Participant: by, Magic: 100},
		&match.LapCompletedEvent{EventMeta: meta, Participant: by, Magic: 300},
		&match.TurnEndedEvent{EventMeta: meta, Participant: by},
		&match.MatchEndedEvent{EventMeta: meta, Winners: indexes(t, 0, 1), Ranks: []int{1, 1}, Reason: match.EndReasonRoundLimit},
		&match.RewardDrawnEvent{EventMeta: meta, Participant: by, Pack: match.DefaultPackKey, Cards: refs(t, 301, 302)},
		&match.AllRewardsReceivedEvent{EventMeta: meta},
		&match.TollPaidEvent{EventMeta: meta, Participant: by, Magic: 60},
		&match.TerritoryRelinquishedEvent{EventMeta: meta, Participant: by, Position: boardtest.Position(t, 1, 0), Magic: 80},
		&match.MagicDepletedEvent{EventMeta: meta, Participant: by, Magic: 600},
		&match.CreaturePlacedEvent{EventMeta: meta, Participant: by, HandIndex: 2, Creature: matchtest.PlacedCreatureID(t, 1), Cost: cardtest.Cost(t, 10)},
		&match.TerritoryInvadedEvent{EventMeta: meta, Participant: by, HandIndex: 2, Cost: cardtest.Cost(t, 10)},
		&match.ItemUsedEvent{EventMeta: meta, Participant: by, HandIndex: 0, Cost: cardtest.Cost(t, 20)},
		&match.CreatureStruckEvent{EventMeta: meta, Striker: match.BattleSideAttacker, Damage: creaturetest.Attack(t, 10), RemainingHitPoints: creaturetest.HitPoints(t, 5)},
		&match.InvasionSucceededEvent{EventMeta: meta, Participant: by, Creature: matchtest.PlacedCreatureID(t, 2), TerritoryHitPoints: creaturetest.HitPoints(t, 20)},
		&match.InvasionFailedEvent{EventMeta: meta, Participant: by, TerritoryHitPoints: creaturetest.HitPoints(t, 15)},
		&match.ItemSkippedEvent{EventMeta: meta, Participant: by},
		&match.LandLeveledUpEvent{EventMeta: meta, Participant: by, Position: boardtest.Position(t, 1, 0), Level: 3, Magic: 300},
		&match.CreatureMovedEvent{EventMeta: meta, Participant: by, From: boardtest.Position(t, 1, 0), To: boardtest.Position(t, 2, 0)},
		&match.TerritoryInvadedByMoveEvent{EventMeta: meta, Participant: by, From: boardtest.Position(t, 1, 0), Position: boardtest.Position(t, 2, 0)},
		&match.LandElementChangedEvent{EventMeta: meta, Participant: by, Position: boardtest.Position(t, 1, 0), Element: element.LandWater, Magic: 200},
		&match.CreatureExchangedEvent{EventMeta: meta, Participant: by, Position: boardtest.Position(t, 1, 0), HandIndex: 0, Creature: matchtest.PlacedCreatureID(t, 3), Cost: cardtest.Cost(t, 10)},
		&match.BattleStartedEvent{EventMeta: meta, Input: goldenBattleInput(t)},
		&match.BattleSupportEffectActivatedEvent{EventMeta: meta, Side: match.BattleSideAttacker, AttackDelta: 20},
		&match.BattleTerrainEffectActivatedEvent{EventMeta: meta, HitPointsDelta: 10},
		&match.BattleCreatureEffectActivatedEvent{EventMeta: meta, Side: match.BattleSideDefender, Effect: effect.KindHitPointsModifier, Change: &match.BattleStatChange{HitPointsDelta: 20}},
		&match.BattleItemEffectActivatedEvent{EventMeta: meta, Side: match.BattleSideAttacker, Effect: effect.KindAttackMultiplier, Change: &match.BattleStatChange{AttackDelta: 20}},
		&match.TollCreatureEffectActivatedEvent{EventMeta: meta},
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
	require.Equal(t, shuffled.DrawPile[:1], state.Participants[1].Hand[len(state.Participants[1].Hand)-1:])
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
	require.Equal(t, 1, events[0].(*match.CardDiscardedEvent).HandIndex)
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

func TestEvolve_RejectsDrawnCardFromAnEmptyDrawPile(t *testing.T) {
	state := withPiles(t, newMatch(t, lineBoard(t), defaultRules(t)), 1, nil, nil)

	_, err := match.Evolve(state, &match.CardDrawnEvent{Participant: matchtest.Index(t, 1)})

	require.ErrorIs(t, err, match.ErrInvalidState)
}

func TestEvolve_RejectsEventOfParticipantWhoseTurnItIsNot(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))
	other := matchtest.Index(t, 0)
	tests := map[string]match.Event{
		"card_drawn":        &match.CardDrawnEvent{Participant: other},
		"card_discarded":    &match.CardDiscardedEvent{Participant: other, HandIndex: 0},
		"dice_rolled":       &match.DiceRolledEvent{Participant: other, Value: 1},
		"moved":             &match.PieceMovedEvent{Participant: other, To: pos(t, 1, 0)},
		"checkpoint_passed": &match.CheckpointPassedEvent{Participant: other, Magic: 100},
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
	require.Equal(t, by, rolled[1].(*match.PieceMovedEvent).Participant)
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
	require.Equal(t, match.PhasePlacing, state.Phase)
	require.Equal(t, 3, countOf[*match.PieceMovedEvent](events))
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
	require.IsType(t, &match.AllRewardsReceivedEvent{}, events[1])
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
		"completed_before_everyone": {received, &match.AllRewardsReceivedEvent{EventMeta: meta}},
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
func placingMatch(t *testing.T, participants ...match.Participant) match.Match {
	t.Helper()
	cmd := startCommand(t, defaultRules(t), participants...)
	state, _ := start(t, cmd)
	state, _ = drawAndRoll(t, state, 1, 1)
	require.Equal(t, match.PhasePlacing, state.Phase)
	return state
}

func TestDecide_StoppingOnVacantLandOffersTheCreaturesThatCanBePaid(t *testing.T) {
	// NPC のデッキ: カード 201（コスト 10）と 299（コスト 990。開始時の魔力 600 では払えない）。山札は逆順に混ざるので、
	// 配った 1 枚が 299、引いた 1 枚が 201
	npc := match.NewNPCParticipant(npcavatartest.ID(t, 7), npcdecktest.ID(t, 8), []match.DeckCard{deckCard(t, 201, 1), deckCard(t, 299, 1)})
	state := placingMatch(t, playerParticipant(t), npc)

	require.Equal(t, refs(t, 299, 201), state.Participants[1].Hand)
	require.Equal(t, []int{1}, state.PlaceChoices())
}

func TestDecide_PlacePaysTheCostMakesTheTerritoryAndEndsTheTurn(t *testing.T) {
	state := placingMatch(t, playerParticipant(t), npcParticipant(t))
	handBefore := len(state.Participants[1].Hand)

	state, events := apply(t, state, matchtest.NewRandom(), &match.PlaceCreatureCommand{By: matchtest.Index(t, 1), HandIndex: 0})

	require.Equal(t, 1, countOf[*match.CreaturePlacedEvent](events))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
	require.Equal(t, 600-10, state.Participants[1].Magic)
	require.Len(t, state.Participants[1].Hand, handBefore-1)
	require.Equal(t, []match.Territory{{Position: pos(t, 1, 0), Creature: matchtest.PlacedCreatureID(t, 1)}}, state.Territories)
	require.Equal(t, []match.PlacedCreature{{
		ID: matchtest.PlacedCreatureID(t, 1), Card: cardtest.CreatureRef(t, 201), Owner: matchtest.Index(t, 1),
		Base: *creatureSpec(t, 201), Attack: creatureSpec(t, 201).Attack, HitPoints: creatureSpec(t, 201).HitPoints,
	}}, state.PlacedCreatures)
	// 次の番（プレイヤー）に移っている
	turn, _ := state.CurrentParticipant()
	require.Equal(t, matchtest.Index(t, 0), turn)
}

func TestDecide_EndTurnWhilePlacingLeavesTheLandVacant(t *testing.T) {
	state := placingMatch(t, playerParticipant(t), npcParticipant(t))

	state, events := endTurn(t, state, 1)

	require.Zero(t, countOf[*match.CreaturePlacedEvent](events))
	require.Empty(t, state.Territories)
	require.Equal(t, 600, state.Participants[1].Magic)
}

func TestDecide_PlaceRejectsCardsThatCannotBePlaced(t *testing.T) {
	itemSpec := match.CardSpec{Cost: cardtest.Cost(t, 10), Rarity: card.RarityNormal}
	item := match.DeckCard{Card: cardtest.Ref(t, card.ItemEntityType, 250), Quantity: cardtest.DeckQuantity(t, 1), Spec: itemSpec}
	// 山札は逆順に混ざるので、手札は先頭から 299・250（アイテム）・201
	npc := match.NewNPCParticipant(npcavatartest.ID(t, 7), npcdecktest.ID(t, 8), []match.DeckCard{deckCard(t, 201, 1), item, deckCard(t, 299, 1)})
	state := placingMatch(t, playerParticipant(t), npc)
	hand := state.Participants[1].Hand
	require.Equal(t, cardtest.CreatureRef(t, 299), hand[0])
	require.Equal(t, cardtest.Ref(t, card.ItemEntityType, 250), hand[1])

	tests := []struct {
		name      string
		handIndex int
		want      error
	}{
		{name: "cost_exceeds_magic", handIndex: 0, want: match.ErrCreatureNotPlaceable},
		{name: "not_creature", handIndex: 1, want: match.ErrCreatureNotPlaceable},
		{name: "out_of_range", handIndex: len(hand), want: match.ErrHandIndexOutOfRange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := decideErr(t, state, &match.PlaceCreatureCommand{By: matchtest.Index(t, 1), HandIndex: tt.handIndex})
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestDecide_StoppingOnATerritoryDoesNotOfferPlacing(t *testing.T) {
	state := placingMatch(t, playerParticipant(t), npcParticipant(t))
	state, _ = apply(t, state, matchtest.NewRandom(), &match.PlaceCreatureCommand{By: matchtest.Index(t, 1), HandIndex: 0})

	// プレイヤーも目 1 で、NPC の領地になった (1,0) に止まる
	state, _ = drawAndRoll(t, state, 0, 1)

	require.Equal(t, pos(t, 1, 0), state.Participants[0].Position)
	require.Equal(t, match.PhaseInvading, state.Phase)
	require.Empty(t, state.PlaceChoices())
}

func TestDecide_PlaceRejectsOutsideThePlacingPhase(t *testing.T) {
	state := newMatch(t, lineBoard(t), defaultRules(t))

	err := decideErr(t, state, &match.PlaceCreatureCommand{By: matchtest.Index(t, 1), HandIndex: 0})

	require.ErrorIs(t, err, match.ErrPhaseNotAllowed)
}

func TestDecide_PlacedCreaturesGetConsecutiveIDs(t *testing.T) {
	state := newMatch(t, ringBoard(t), defaultRules(t))
	// 拠点からの最初の一歩は 2 方向に分かれる。NPC は (1,0) へ進んで目 2 で土地 (2,0)、プレイヤーは (0,1) へ進んで目 1 で土地 (0,1)
	state, _ = drawAndRoll(t, state, 1, 2)
	state, _ = choose(t, state, 1, 1, 0)
	state, _ = apply(t, state, matchtest.NewRandom(), &match.PlaceCreatureCommand{By: matchtest.Index(t, 1), HandIndex: 0})
	state, _ = drawAndRoll(t, state, 0, 1)
	state, _ = choose(t, state, 0, 0, 1)
	state, _ = apply(t, state, matchtest.NewRandom(), &match.PlaceCreatureCommand{By: matchtest.Index(t, 0), HandIndex: 0})

	require.Equal(t, []match.Territory{
		{Position: pos(t, 2, 0), Creature: matchtest.PlacedCreatureID(t, 1)},
		{Position: pos(t, 0, 1), Creature: matchtest.PlacedCreatureID(t, 2)},
	}, state.Territories)
	require.Equal(t, matchtest.Index(t, 0), state.PlacedCreatures[1].Owner)
	require.Equal(t, matchtest.PlacedCreatureID(t, 2), state.LastPlacedCreatureID)
}

func TestEvolve_RejectsPlacedEventThatDoesNotMatchTheState(t *testing.T) {
	state := placingMatch(t, playerParticipant(t), npcParticipant(t))
	meta := match.EventMeta{EntityID: state.ID, AggregateVersion: state.Version + 1, OccurredAt: startedAt}
	valid := match.CreaturePlacedEvent{EventMeta: meta, Participant: matchtest.Index(t, 1), HandIndex: 0, Creature: matchtest.PlacedCreatureID(t, 1), Cost: cardtest.Cost(t, 10)}

	tests := []struct {
		name   string
		mutate func(e *match.CreaturePlacedEvent)
	}{
		{name: "hand_index_out_of_range", mutate: func(e *match.CreaturePlacedEvent) { e.HandIndex = 99 }},
		{name: "skipped_id", mutate: func(e *match.CreaturePlacedEvent) { e.Creature = matchtest.PlacedCreatureID(t, 2) }},
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
	id := matchtest.PlacedCreatureID(t, state.LastPlacedCreatureID.Int()+1)
	spec := creatureSpec(t, 201)
	state.PlacedCreatures = append(slices.Clone(state.PlacedCreatures), match.PlacedCreature{
		ID: id, Card: cardtest.CreatureRef(t, 201), Owner: matchtest.Index(t, owner), Base: *spec, HitPoints: spec.HitPoints,
	})
	state.Territories = append(slices.Clone(state.Territories), match.Territory{Position: pos(t, x, 0), Creature: id})
	if level > 1 {
		state.LandLevels = append(slices.Clone(state.LandLevels), match.LandLevel{Position: pos(t, x, 0), Level: level})
	}
	state.LastPlacedCreatureID = id
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

// 領地のクリーチャーに通行料の倍率を持たせる
func withTollMultiplier(t *testing.T, state match.Match, percent int) match.Match {
	t.Helper()
	effects := effecttest.Effects(t, effecttest.TollMultiplier(t, percent))
	state.PlacedCreatures = slices.Clone(state.PlacedCreatures)
	for i := range state.PlacedCreatures {
		state.PlacedCreatures[i].Base.Effects = effects
	}
	return state
}

func TestMatch_TerritoryTollDoesNotApplyTheTollMultiplier(t *testing.T) {
	state := withTollMultiplier(t, withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1), 150)
	territory, ok := state.TerritoryAt(pos(t, 1, 0))
	require.True(t, ok)

	// 盤面の通行料は価値 100 × 60% のままで、倍率は払うときに発動する
	require.Equal(t, 60, state.TerritoryToll(territory))
	require.Equal(t, 100, state.TerritoryValue(territory))
}

func TestDecide_PayTollActivatesTheTollMultiplierBeforePaying(t *testing.T) {
	tests := []struct {
		name    string
		percent int
		want    int
	}{
		// 盤面の通行料 60 × 150% = 90
		{name: "one_and_a_half", percent: 150, want: 90},
		// 60 × 133% = 79.8 → 79（盤面の通行料に掛けてから切り捨てる）
		{name: "floors", percent: 133, want: 79},
		// 効き目が無くても、持っていれば発動する
		{name: "no_effect", percent: 100, want: 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := withTollMultiplier(t, withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1), tt.percent)
			state, _ = drawAndRoll(t, state, 1, 1)

			state, events := payToll(t, state, 1)

			require.Equal(t, 1, countOf[*match.TollCreatureEffectActivatedEvent](events))
			require.Equal(t, tt.want, eventOf[*match.TollPaidEvent](t, events).Magic)
			require.Less(t, slices.IndexFunc(events, isEvent[*match.TollCreatureEffectActivatedEvent]), slices.IndexFunc(events, isEvent[*match.TollPaidEvent]))
			require.Equal(t, 600-tt.want, state.Participants[1].Magic)
		})
	}
}

func TestDecide_PayTollWithoutTheTollMultiplierDoesNotActivateIt(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1)
	state, _ = drawAndRoll(t, state, 1, 1)

	_, events := payToll(t, state, 1)

	require.Zero(t, countOf[*match.TollCreatureEffectActivatedEvent](events))
	require.Equal(t, 60, eventOf[*match.TollPaidEvent](t, events).Magic)
}

func TestDecide_FailedInvasionActivatesTheTollMultiplierOfTheDefender(t *testing.T) {
	state := invade(t, withTollMultiplier(t, invadingMatch(t), 150))

	state, events := defend(t, state)

	require.Equal(t, 1, countOf[*match.TollCreatureEffectActivatedEvent](events))
	require.Equal(t, 90, eventOf[*match.TollPaidEvent](t, events).Magic)
	require.Equal(t, 600-10-90, state.Participants[1].Magic)
}

func isEvent[T match.Event](event match.Event) bool {
	_, ok := event.(T)
	return ok
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
	require.Equal(t, match.PhasePlacing, state.Phase)
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

func TestDecide_PlacingOnALeveledVacantLandInheritsTheLevel(t *testing.T) {
	state := territoryBoardMatch(t)
	state.LandLevels = []match.LandLevel{{Position: pos(t, 1, 0), Level: 3}}
	state = rolledTo(t, state, 1)
	require.Equal(t, match.PhasePlacing, state.Phase)

	state, _ = apply(t, state, matchtest.NewRandom(), &match.PlaceCreatureCommand{By: matchtest.Index(t, 1), HandIndex: 0})

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
	_, placed := state.PlacedCreature(matchtest.PlacedCreatureID(t, 2))
	require.False(t, placed)
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
	valid := match.TerritoryRelinquishedEvent{EventMeta: meta, Participant: matchtest.Index(t, 1), Position: pos(t, 2, 0), Magic: 80}

	tests := []struct {
		name   string
		mutate func(e *match.TerritoryRelinquishedEvent)
	}{
		{name: "other_position", mutate: func(e *match.TerritoryRelinquishedEvent) { e.Position = pos(t, 3, 0) }},
		{name: "territory_of_another_owner", mutate: func(e *match.TerritoryRelinquishedEvent) { e.Position = pos(t, 1, 0) }},
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

// 手札の最後に、効果の無い武器を足す（デッキの写しにも足す。性能は写しから読むため）
func withItemInHand(t *testing.T, state match.Match, participant int, id int64, cost int) match.Match {
	t.Helper()
	ref := cardtest.Ref(t, card.ItemEntityType, id)
	itemSpec := &match.ItemSpec{Category: card.ItemCategoryWeapon, Effects: effecttest.Effects(t)}
	spec := match.CardSpec{Cost: cardtest.Cost(t, cost), Rarity: card.RarityNormal, Item: itemSpec}
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
	invasion.Attack = invasion.Base.Attack
	invasion.HitPoints = creaturetest.HitPoints(t, attackerHitPoints)
	state.Invasion = &invasion
	state.PlacedCreatures = slices.Clone(state.PlacedCreatures)
	state.PlacedCreatures[0].Base.Attack = creaturetest.Attack(t, defenderAttack)
	state.PlacedCreatures[0].Attack = state.PlacedCreatures[0].Base.Attack
	state.PlacedCreatures[0].HitPoints = creaturetest.HitPoints(t, defenderHitPoints)
	return state
}

// 倒れた側の破壊（Destroyed）が、侵略の結果（Result）の直前に 1 つだけある
func requireDestroyedBefore[Destroyed, Result match.Event](t *testing.T, events []match.Event) {
	t.Helper()
	for i, event := range events {
		if _, ok := event.(Result); ok {
			require.Positive(t, i)
			require.IsType(t, *new(Destroyed), events[i-1])
			require.Equal(t, 1, countOf[*match.InvadingCreatureDestroyedEvent](events)+countOf[*match.DefendingCreatureDestroyedEvent](events))
			return
		}
	}
	require.FailNow(t, "result event not found")
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
	require.NotContains(t, state.Participants[1].Hand, cardtest.Ref(t, card.ItemEntityType, 301))
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

	conquered := eventOf[*match.InvasionSucceededEvent](t, events)
	// 破壊は侵略の結果に埋めず、結果の直前に戦闘とスペルで共通のイベントで出す（core#181）
	requireDestroyedBefore[*match.DefendingCreatureDestroyedEvent, *match.InvasionSucceededEvent](t, events)
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
	require.Zero(t, countOf[*match.TollPaidEvent](events))
	require.Nil(t, state.Invasion)
	require.Equal(t, []match.Territory{{Position: pos(t, 1, 0), Creature: conquered.Creature}}, state.Territories)
	require.Len(t, state.PlacedCreatures, 1)
	require.Equal(t, matchtest.Index(t, 1), state.PlacedCreatures[0].Owner)
	require.Equal(t, cardtest.CreatureRef(t, 201), state.PlacedCreatures[0].Card)
	require.Equal(t, creaturetest.HitPoints(t, 20), state.PlacedCreatures[0].HitPoints)
	require.Equal(t, state.LastPlacedCreatureID, conquered.Creature)
	require.NotEqual(t, defeated, conquered.Creature)
	require.Equal(t, []card.Ref{cardtest.CreatureRef(t, 201)}, state.Participants[0].DiscardPile)
	require.Empty(t, state.Participants[1].DiscardPile)
}

func TestDecide_FailingToDefeatTheDefenderReturnsTheSurvivorToTheHandAndPaysTheToll(t *testing.T) {
	state := invade(t, invadingMatch(t))
	handBefore := len(state.Participants[1].Hand)

	state, events := defend(t, state)

	// 攻撃 5・体力 20 どうしなので、どちらも倒れない
	require.NotNil(t, eventOf[*match.InvasionFailedEvent](t, events))
	require.Zero(t, countOf[*match.InvadingCreatureDestroyedEvent](events)+countOf[*match.DefendingCreatureDestroyedEvent](events))
	require.Len(t, state.Participants[1].Hand, handBefore+1)
	require.Equal(t, cardtest.CreatureRef(t, 201), state.Participants[1].Hand[handBefore])
	require.Equal(t, 600-10-60, state.Participants[1].Magic)
	require.Equal(t, 600+60, state.Participants[0].Magic)
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
	require.Nil(t, state.Invasion)
	// 傷は残る
	require.Equal(t, creaturetest.HitPoints(t, 15), state.PlacedCreatures[0].HitPoints)
}

func TestDecide_AttackerDestroyedByTheCounterattackIsNotReturnedToTheHand(t *testing.T) {
	state := withBattlers(t, invade(t, invadingMatch(t)), 5, 10, 10, 20)
	handBefore := len(state.Participants[1].Hand)

	state, events := defend(t, state)

	requireDestroyedBefore[*match.InvadingCreatureDestroyedEvent, *match.InvasionFailedEvent](t, events)
	require.Len(t, state.Participants[1].Hand, handBefore)
	require.Equal(t, []card.Ref{cardtest.CreatureRef(t, 201)}, state.Participants[1].DiscardPile)
	require.Equal(t, 1, countOf[*match.TollPaidEvent](events))
}

// 侵略したクリーチャーと領地のクリーチャーに効果を持たせる
func withCreatureEffects(t *testing.T, state match.Match, attacker, defender []effect.Effect) match.Match {
	t.Helper()
	invasion := *state.Invasion
	invasion.Base.Effects = effecttest.Effects(t, attacker...)
	state.Invasion = &invasion
	state.PlacedCreatures = slices.Clone(state.PlacedCreatures)
	state.PlacedCreatures[0].Base.Effects = effecttest.Effects(t, defender...)
	return state
}

// 戦闘の出来事を 1 行ずつの文字列に縮める（期待する並びをそのまま表に書けるように）。戦闘の開始は入力そのものなので除く。
// 効果は「側 出どころ AT の増減 HP の増減 → 当てた後の AT/HP」、発動は「側 activates 能力」、攻撃は「攻撃した側 strikes ダメージ → 受けた側の残りの HP」
// 戦闘の出来事を 1 行ずつにする。発動のイベントは当てた後の値を持たないので、入力の戦闘の前の値から増減を足して添える
func battleLines(events []match.Event) []string {
	var lines []string
	stats := map[match.BattleSide][2]int{}
	terrain := 0
	modified := func(side match.BattleSide, source string, change match.BattleStatChange) string {
		current := stats[side]
		current = [2]int{max(current[0]+change.AttackDelta, 0), max(current[1]+change.HitPointsDelta, 0)}
		stats[side] = current
		return fmt.Sprintf("%s %s AT%+d HP%+d → %d/%d", side, source, change.AttackDelta, change.HitPointsDelta, current[0], current[1])
	}
	for _, event := range events {
		switch e := event.(type) {
		case *match.BattleStartedEvent:
			stats[match.BattleSideAttacker] = [2]int{e.Input.Attacker.Attack.Int(), e.Input.Attacker.HitPoints.Int()}
			stats[match.BattleSideDefender] = [2]int{e.Input.Defender.Attack.Int(), e.Input.Defender.HitPoints.Int()}
		case *match.BattleSupportEffectActivatedEvent:
			lines = append(lines, modified(e.Side, "adjacent_territories", match.BattleStatChange{AttackDelta: e.AttackDelta}))
		case *match.BattleTerrainEffectActivatedEvent:
			terrain = e.HitPointsDelta
			lines = append(lines, modified(match.BattleSideDefender, "terrain", match.BattleStatChange{HitPointsDelta: e.HitPointsDelta}))
		case *match.BattleItemEffectActivatedEvent:
			if e.Change != nil {
				lines = append(lines, modified(e.Side, "item:"+string(e.Effect), *e.Change))
				break
			}
			// 地形効果を無視する: 相手の HP から地形効果の分を除く
			opponent := match.BattleSideDefender
			if e.Side == match.BattleSideDefender {
				opponent = match.BattleSideAttacker
			}
			current := stats[opponent]
			stats[opponent] = [2]int{current[0], max(current[1]-terrain, 0)}
			lines = append(lines, fmt.Sprintf("%s activates item:%s → %s %d/%d", e.Side, e.Effect, opponent, stats[opponent][0], stats[opponent][1]))
		case *match.BattleCreatureEffectActivatedEvent:
			if e.Change != nil {
				lines = append(lines, modified(e.Side, "creature:"+string(e.Effect), *e.Change))
			} else {
				lines = append(lines, fmt.Sprintf("%s activates %s", e.Side, e.Effect))
			}
		case *match.CreatureStruckEvent:
			receiver := match.BattleSideDefender
			if e.Striker == match.BattleSideDefender {
				receiver = match.BattleSideAttacker
			}
			current := stats[receiver]
			stats[receiver] = [2]int{current[0], e.RemainingHitPoints.Int()}
			lines = append(lines, fmt.Sprintf("%s strikes %d → %d", e.Striker, e.Damage.Int(), e.RemainingHitPoints.Int()))
		}
	}
	return lines
}

// 戦闘の決まり（効果の順・効き方・下限・途中の決着・攻撃の順・能力）は、入力と出来事の並びの表で確かめる。
// 能力やルールを足したら、ここに行を足す（core#172）
func TestResolveBattle_ProducesTheEventsFromTheInput(t *testing.T) {
	attacker, defender := match.BattleSideAttacker, match.BattleSideDefender
	first, last := effect.NewStrikeFirstEffect(), effect.NewStrikeLastEffect()
	random := randomStats(t, 10, 70)
	immunity := effecttest.NullifyReceivedDamage(t, "fire", "earth")
	attackModifier, hitPointsModifier := effecttest.AttackModifier, effecttest.HitPointsModifier
	cases := []struct {
		name  string
		input *matchtest.BattleInputBuilder
		// 乱数の元が返す値（IntN の戻り値）。無ければ 0 を返す
		random []int
		want   []string
	}{
		{name: "both_survive", input: matchtest.Battle(t),
			want: []string{"attacker strikes 10 → 30", "defender strikes 10 → 30"}},
		{name: "defeated_defender_does_not_strike_back", input: matchtest.Battle(t).Stats(attacker, 30, 20).Stats(defender, 50, 30),
			want: []string{"attacker strikes 30 → 0"}},
		{name: "effects_of_the_attacker_then_the_defender_before_the_strikes",
			input: matchtest.Battle(t).Stats(attacker, 10, 40).Stats(defender, 20, 40).Elements(defender, "fire").Land(element.LandFire, 2).AdjacentTerritories(attacker, 2).
				Item(attacker, card.ItemCategoryWeapon, attackModifier(25)).Item(defender, card.ItemCategoryArmor, hitPointsModifier(15)),
			want: []string{
				"attacker adjacent_territories AT+20 HP+0 → 30/40", "attacker item:attack_modifier AT+25 HP+0 → 55/40",
				"defender terrain AT+0 HP+20 → 20/60", "defender item:hit_points_modifier AT+0 HP+15 → 20/75",
				"attacker strikes 55 → 20", "defender strikes 20 → 20",
			}},
		{name: "creature_effects_come_first_on_each_side",
			input: matchtest.Battle(t).Stats(attacker, 10, 40).Stats(defender, 20, 40).Elements(defender, "fire").Land(element.LandFire, 2).AdjacentTerritories(attacker, 2).
				Item(attacker, card.ItemCategoryWeapon, attackModifier(25)).Item(defender, card.ItemCategoryArmor, hitPointsModifier(15)).Effects(attacker, attackModifier(5)).Effects(defender, hitPointsModifier(30)),
			want: []string{
				"attacker creature:attack_modifier AT+5 HP+0 → 15/40", "attacker adjacent_territories AT+20 HP+0 → 35/40", "attacker item:attack_modifier AT+25 HP+0 → 60/40",
				"defender creature:hit_points_modifier AT+0 HP+30 → 20/70", "defender terrain AT+0 HP+20 → 20/90", "defender item:hit_points_modifier AT+0 HP+15 → 20/105",
				"attacker strikes 60 → 45", "defender strikes 20 → 20",
			}},
		{name: "terrain_with_the_same_element", input: matchtest.Battle(t).Elements(defender, "fire").Land(element.LandFire, 1),
			want: []string{"defender terrain AT+0 HP+10 → 10/50", "attacker strikes 10 → 40", "defender strikes 10 → 30"}},
		{name: "terrain_with_one_of_the_elements", input: matchtest.Battle(t).Elements(defender, "fire", "water").Land(element.LandFire, 1),
			want: []string{"defender terrain AT+0 HP+10 → 10/50", "attacker strikes 10 → 40", "defender strikes 10 → 30"}},
		{name: "terrain_with_another_element", input: matchtest.Battle(t).Elements(defender, "water").Land(element.LandFire, 1),
			want: []string{"attacker strikes 10 → 30", "defender strikes 10 → 30"}},
		{name: "terrain_of_a_multi_land_with_an_element", input: matchtest.Battle(t).Elements(defender, "water").Land(element.LandMulti, 1),
			want: []string{"defender terrain AT+0 HP+10 → 10/50", "attacker strikes 10 → 40", "defender strikes 10 → 30"}},
		{name: "terrain_of_a_multi_land_with_neutral", input: matchtest.Battle(t).Elements(defender, "neutral").Land(element.LandMulti, 1),
			want: []string{"attacker strikes 10 → 30", "defender strikes 10 → 30"}},
		{name: "terrain_of_a_neutral_land_with_neutral", input: matchtest.Battle(t).Elements(defender, "neutral").Land(element.LandNeutral, 1),
			want: []string{"defender terrain AT+0 HP+10 → 10/50", "attacker strikes 10 → 40", "defender strikes 10 → 30"}},
		{name: "counterattack_destroys_the_attacker", input: matchtest.Battle(t).Stats(attacker, 10, 10),
			want: []string{"attacker strikes 10 → 30", "defender strikes 10 → 0"}},
		{name: "item_effects_apply_in_order", input: matchtest.Battle(t).Item(attacker, card.ItemCategoryWeapon, attackModifier(10), hitPointsModifier(20)),
			want: []string{"attacker item:attack_modifier AT+10 HP+0 → 20/40", "attacker item:hit_points_modifier AT+0 HP+20 → 20/60", "attacker strikes 20 → 20", "defender strikes 10 → 50"}},
		{name: "support_effect_of_the_defender_before_the_terrain_effect", input: matchtest.Battle(t).Elements(defender, "fire").Land(element.LandFire, 1).AdjacentTerritories(defender, 1),
			want: []string{"defender adjacent_territories AT+10 HP+0 → 20/40", "defender terrain AT+0 HP+10 → 20/50", "attacker strikes 10 → 40", "defender strikes 20 → 20"}},
		{name: "bonuses_follow_the_values_of_the_rules", input: matchtest.Battle(t).Elements(defender, "fire").Land(element.LandFire, 2).AdjacentTerritories(attacker, 2).Rules(15, 5),
			want: []string{"attacker adjacent_territories AT+30 HP+0 → 40/40", "defender terrain AT+0 HP+10 → 10/50", "attacker strikes 40 → 10", "defender strikes 10 → 30"}},
		{name: "attack_override_sets_the_attack", input: matchtest.Battle(t).Item(attacker, card.ItemCategoryScroll, effecttest.AttackOverride(t, 30)),
			want: []string{"attacker item:attack_override AT+20 HP+0 → 30/40", "attacker strikes 30 → 10", "defender strikes 10 → 30"}},
		{name: "attack_override_after_the_other_effects_of_the_side", input: matchtest.Battle(t).AdjacentTerritories(attacker, 1).Effects(attacker, attackModifier(5)).Item(attacker, card.ItemCategoryScroll, effecttest.AttackOverride(t, 30)),
			want: []string{
				"attacker creature:attack_modifier AT+5 HP+0 → 15/40", "attacker adjacent_territories AT+10 HP+0 → 25/40", "attacker item:attack_override AT+5 HP+0 → 30/40",
				"attacker strikes 30 → 10", "defender strikes 10 → 30",
			}},
		{name: "attack_multiplier_against_a_matching_opponent", input: matchtest.Battle(t).Elements(defender, "water").Item(attacker, card.ItemCategoryWeapon, attackModifier(25), effecttest.AttackMultiplier(t, 150, "water")),
			want: []string{"attacker item:attack_modifier AT+25 HP+0 → 35/40", "attacker item:attack_multiplier AT+17 HP+0 → 52/40", "attacker strikes 52 → 0"}},
		{name: "attack_multiplier_against_another_element", input: matchtest.Battle(t).Elements(defender, "earth").Item(attacker, card.ItemCategoryWeapon, attackModifier(25), effecttest.AttackMultiplier(t, 150, "water")),
			want: []string{"attacker item:attack_modifier AT+25 HP+0 → 35/40", "attacker strikes 35 → 5", "defender strikes 10 → 30"}},
		{name: "attack_multiplier_after_the_effects_of_both_sides", input: matchtest.Battle(t).Elements(defender, "water").Item(attacker, card.ItemCategoryWeapon, effecttest.AttackMultiplier(t, 150, "water")).Item(defender, card.ItemCategoryArmor, hitPointsModifier(15)),
			want: []string{"defender item:hit_points_modifier AT+0 HP+15 → 10/55", "attacker item:attack_multiplier AT+5 HP+0 → 15/40", "attacker strikes 15 → 40", "defender strikes 10 → 30"}},
		{name: "only_the_highest_attack_multiplier_applies", input: matchtest.Battle(t).Elements(defender, "water").Item(attacker, card.ItemCategoryWeapon, effecttest.AttackMultiplier(t, 150, "water"), effecttest.AttackMultiplier(t, 200, "water", "fire")),
			want: []string{"attacker item:attack_multiplier AT+10 HP+0 → 20/40", "attacker strikes 20 → 20", "defender strikes 10 → 30"}},
		{name: "attack_multiplier_of_the_defender", input: matchtest.Battle(t).Elements(attacker, "fire").Item(defender, card.ItemCategoryWeapon, effecttest.AttackMultiplier(t, 300, "fire")),
			want: []string{"defender item:attack_multiplier AT+20 HP+0 → 30/40", "attacker strikes 10 → 30", "defender strikes 30 → 10"}},
		{name: "scroll_ignores_the_terrain_effect_of_the_defender",
			input: matchtest.Battle(t).Elements(defender, "fire").Land(element.LandFire, 2).
				Item(attacker, card.ItemCategoryScroll, effecttest.AttackOverride(t, 30), effect.NewIgnoreTerrainEffect(), effecttest.AttackMultiplier(t, 150, "fire")),
			want: []string{
				"attacker item:attack_override AT+20 HP+0 → 30/40", "defender terrain AT+0 HP+20 → 10/60", "attacker item:attack_multiplier AT+15 HP+0 → 45/40",
				"attacker activates item:ignore_terrain_effect → defender 10/40", "attacker strikes 45 → 0",
			}},
		{name: "ignore_terrain_effect_without_a_terrain_effect", input: matchtest.Battle(t).Item(attacker, card.ItemCategoryScroll, effecttest.AttackOverride(t, 30), effect.NewIgnoreTerrainEffect()),
			want: []string{"attacker item:attack_override AT+20 HP+0 → 30/40", "attacker strikes 30 → 10", "defender strikes 10 → 30"}},
		{name: "ignore_terrain_effect_after_the_defender_strikes_first",
			input: matchtest.Battle(t).Elements(defender, "fire").Land(element.LandFire, 1).Effects(defender, first).Item(attacker, card.ItemCategoryScroll, effecttest.AttackOverride(t, 30), effect.NewIgnoreTerrainEffect()),
			want: []string{
				"attacker item:attack_override AT+20 HP+0 → 30/40", "defender terrain AT+0 HP+10 → 10/50", "defender activates strike_first",
				"defender strikes 10 → 30", "attacker activates item:ignore_terrain_effect → defender 10/40", "attacker strikes 30 → 10",
			}},
		{name: "ignore_terrain_effect_then_nullify_received_damage",
			input: matchtest.Battle(t).Elements(attacker, "fire").Elements(defender, "fire").Land(element.LandFire, 1).Effects(defender, immunity).Item(attacker, card.ItemCategoryScroll, effect.NewIgnoreTerrainEffect()),
			want: []string{
				"defender terrain AT+0 HP+10 → 10/50", "attacker activates item:ignore_terrain_effect → defender 10/40", "defender activates nullify_received_damage",
				"attacker strikes 0 → 40", "defender strikes 10 → 30",
			}},
		// アイテムの増減は、クリーチャーの効果と同じく増減が 0 でも発動する（core#175）
		{name: "item_without_a_change_activates", input: matchtest.Battle(t).Item(attacker, card.ItemCategoryWeapon, attackModifier(0)),
			want: []string{"attacker item:attack_modifier AT+0 HP+0 → 10/40", "attacker strikes 10 → 30", "defender strikes 10 → 30"}},
		{name: "attacker_destroyed_by_an_effect_ends_the_battle", input: matchtest.Battle(t).Stats(attacker, 50, 20).Item(attacker, card.ItemCategoryArmor, hitPointsModifier(-99)),
			want: []string{"attacker item:hit_points_modifier AT+0 HP-20 → 50/0"}},
		// 守る側の効果は当たらず、乱数も引かない（引けば乱数の元が尽きて落ちる）
		{name: "effects_after_the_attacker_is_destroyed_are_skipped", input: matchtest.Battle(t).Item(attacker, card.ItemCategoryArmor, hitPointsModifier(-99)).Effects(defender, random...),
			want: []string{"attacker item:hit_points_modifier AT+0 HP-40 → 10/0"}},
		{name: "defender_destroyed_by_an_effect_ends_the_battle", input: matchtest.Battle(t).Item(defender, card.ItemCategoryArmor, hitPointsModifier(-99)),
			want: []string{"defender item:hit_points_modifier AT+0 HP-40 → 10/0"}},
		{name: "attack_reduced_to_zero_does_not_destroy", input: matchtest.Battle(t).Item(defender, card.ItemCategoryWeapon, attackModifier(-50)),
			want: []string{"defender item:attack_modifier AT-10 HP+0 → 0/40", "attacker strikes 10 → 30", "defender strikes 0 → 40"}},
		{name: "defender_strike_first", input: matchtest.Battle(t).Effects(defender, first),
			want: []string{"defender activates strike_first", "defender strikes 10 → 30", "attacker strikes 10 → 30"}},
		{name: "attacker_strike_first", input: matchtest.Battle(t).Effects(attacker, first),
			want: []string{"attacker activates strike_first", "attacker strikes 10 → 30", "defender strikes 10 → 30"}},
		{name: "both_strike_first", input: matchtest.Battle(t).Effects(attacker, first).Effects(defender, first),
			want: []string{"attacker activates strike_first", "defender activates strike_first", "attacker strikes 10 → 30", "defender strikes 10 → 30"}},
		{name: "attacker_strike_last", input: matchtest.Battle(t).Effects(attacker, last),
			want: []string{"attacker activates strike_last", "defender strikes 10 → 30", "attacker strikes 10 → 30"}},
		{name: "defender_strike_last", input: matchtest.Battle(t).Effects(defender, last),
			want: []string{"defender activates strike_last", "attacker strikes 10 → 30", "defender strikes 10 → 30"}},
		{name: "both_strike_last", input: matchtest.Battle(t).Effects(attacker, last).Effects(defender, last),
			want: []string{"attacker activates strike_last", "defender activates strike_last", "attacker strikes 10 → 30", "defender strikes 10 → 30"}},
		{name: "attacker_strike_last_against_strike_first", input: matchtest.Battle(t).Effects(attacker, last).Effects(defender, first),
			want: []string{"attacker activates strike_last", "defender activates strike_first", "defender strikes 10 → 30", "attacker strikes 10 → 30"}},
		{name: "strike_first_destroys_the_attacker_before_it_strikes", input: matchtest.Battle(t).Stats(attacker, 50, 10).Effects(defender, first),
			want: []string{"defender activates strike_first", "defender strikes 10 → 0"}},
		{name: "nullify_received_damage_against_a_matching_element", input: matchtest.Battle(t).Elements(attacker, "fire").Effects(defender, immunity),
			want: []string{"defender activates nullify_received_damage", "attacker strikes 0 → 40", "defender strikes 10 → 30"}},
		{name: "nullify_received_damage_against_one_of_the_elements", input: matchtest.Battle(t).Elements(attacker, "earth", "water").Effects(defender, immunity),
			want: []string{"defender activates nullify_received_damage", "attacker strikes 0 → 40", "defender strikes 10 → 30"}},
		{name: "nullify_received_damage_against_another_element", input: matchtest.Battle(t).Elements(attacker, "water").Effects(defender, immunity),
			want: []string{"attacker strikes 10 → 30", "defender strikes 10 → 30"}},
		{name: "nullify_received_damage_against_neutral", input: matchtest.Battle(t).Elements(attacker, "neutral").Effects(defender, immunity),
			want: []string{"attacker strikes 10 → 30", "defender strikes 10 → 30"}},
		{name: "nullify_received_damage_of_the_attacker", input: matchtest.Battle(t).Elements(defender, "earth").Effects(attacker, immunity),
			want: []string{"attacker strikes 10 → 30", "attacker activates nullify_received_damage", "defender strikes 0 → 40"}},
		{name: "nullify_received_damage_against_raised_attack", input: matchtest.Battle(t).Elements(attacker, "fire").Item(attacker, card.ItemCategoryWeapon, attackModifier(30)).Effects(defender, immunity),
			want: []string{"attacker item:attack_modifier AT+30 HP+0 → 40/40", "defender activates nullify_received_damage", "attacker strikes 0 → 40", "defender strikes 10 → 30"}},
		{name: "creature_modifier_activates_even_when_zero", input: matchtest.Battle(t).Effects(defender, attackModifier(0)),
			want: []string{"defender creature:attack_modifier AT+0 HP+0 → 10/40", "attacker strikes 10 → 30", "defender strikes 10 → 30"}},
		{name: "creature_modifier_lowering_hit_points_to_zero_ends_the_battle", input: matchtest.Battle(t).Effects(defender, hitPointsModifier(-99)),
			want: []string{"defender creature:hit_points_modifier AT+0 HP-40 → 10/0"}},
		{name: "overrides_roll_in_the_order_of_the_effects", input: matchtest.Battle(t).Effects(defender, random...), random: []int{5, 60},
			want: []string{"defender creature:attack_override AT+5 HP+0 → 15/40", "defender creature:hit_points_override AT+0 HP+30 → 15/70", "attacker strikes 10 → 60", "defender strikes 15 → 25"}},
		{name: "override_records_the_difference_as_the_delta", input: matchtest.Battle(t).Stats(defender, 30, 50).Effects(defender, random...), random: []int{0, 60},
			want: []string{"defender creature:attack_override AT-20 HP+0 → 10/50", "defender creature:hit_points_override AT+0 HP+20 → 10/70", "attacker strikes 10 → 60", "defender strikes 10 → 30"}},
		{name: "overrides_of_the_attacker", input: matchtest.Battle(t).Effects(attacker, random...), random: []int{0, 0},
			want: []string{"attacker creature:attack_override AT+0 HP+0 → 10/40", "attacker creature:hit_points_override AT+0 HP-30 → 10/10", "attacker strikes 10 → 30", "defender strikes 10 → 0"}},
		{name: "override_activates_even_when_unchanged", input: matchtest.Battle(t).Effects(defender, random...), random: []int{0, 30},
			want: []string{"defender creature:attack_override AT+0 HP+0 → 10/40", "defender creature:hit_points_override AT+0 HP+0 → 10/40", "attacker strikes 10 → 30", "defender strikes 10 → 30"}},
		{name: "modifiers_then_overrides", input: matchtest.Battle(t).Effects(defender, append([]effect.Effect{attackModifier(100), hitPointsModifier(100)}, random...)...), random: []int{0, 30},
			want: []string{
				"defender creature:attack_modifier AT+100 HP+0 → 110/40", "defender creature:hit_points_modifier AT+0 HP+100 → 110/140",
				"defender creature:attack_override AT-100 HP+0 → 10/140", "defender creature:hit_points_override AT+0 HP-100 → 10/40",
				"attacker strikes 10 → 30", "defender strikes 10 → 30",
			}},
		{name: "overrides_then_modifiers", input: matchtest.Battle(t).Effects(defender, append(random, attackModifier(100), hitPointsModifier(100))...), random: []int{0, 30},
			want: []string{
				"defender creature:attack_override AT+0 HP+0 → 10/40", "defender creature:hit_points_override AT+0 HP+0 → 10/40",
				"defender creature:attack_modifier AT+100 HP+0 → 110/40", "defender creature:hit_points_modifier AT+0 HP+100 → 110/140",
				"attacker strikes 10 → 130", "defender strikes 110 → 0",
			}},
		// 固定の上書き（巻物）は乱数を引かない（引けば乱数の元が尽きて落ちる）
		{name: "fixed_override_does_not_roll", input: matchtest.Battle(t).Effects(defender, effecttest.AttackOverride(t, 30)),
			want: []string{"defender creature:attack_override AT+20 HP+0 → 30/40", "attacker strikes 10 → 30", "defender strikes 30 → 10"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input.Input()

			events, err := match.ResolveBattle(input, matchtest.NewSequenceRandom(tc.random...))

			require.NoError(t, err)
			require.Equal(t, &match.BattleStartedEvent{Input: input}, events[0])
			require.Equal(t, tc.want, battleLines(events))
		})
	}
}

// 発動で得た状態は戦闘の中の値に入り、攻撃の順と攻撃のダメージはその値で決まる（入力の性能を見直さない。core#175）
func TestEvolve_ActivatedBattleEffectsGiveTheirStateToTheBattle(t *testing.T) {
	state := withBattlers(t, invade(t, invadingMatch(t)), 10, 40, 10, 40)
	invasion := *state.Invasion
	invasion.Base.Elements = creaturetest.Elements(t, "fire")
	state.Invasion = &invasion
	immunity := effecttest.NullifyReceivedDamage(t, "fire")
	state = withCreatureEffects(t, state, []effect.Effect{effect.NewStrikeLastEffect()}, []effect.Effect{immunity})
	events, err := match.Decide(state, &match.DefendCommand{By: matchtest.Index(t, 0)}, startedAt, matchtest.NewRandom())
	require.NoError(t, err)

	var next match.State = state
	var activated []match.Battle
	for _, event := range events {
		next, err = match.Evolve(next, event)
		require.NoError(t, err)
		if _, ok := event.(*match.BattleCreatureEffectActivatedEvent); ok {
			activated = append(activated, *next.(match.Match).Battle)
		}
	}

	require.Len(t, activated, 2)
	require.Equal(t, -1, activated[0].Attacker.StrikePriority)
	require.Equal(t, 100, activated[0].Defender.ReceivedDamagePercent)
	require.Equal(t, 0, activated[1].Defender.ReceivedDamagePercent)
	// 後に攻撃する攻める側は後に攻撃し、守る側の受けるダメージを 0 にするで受けた量は 0
	require.Equal(t, []string{"attacker activates strike_last", "defender strikes 10 → 30", "defender activates nullify_received_damage", "attacker strikes 0 → 40"}, battleLines(events))
}

func TestDecide_HitPointsAddedByBattleModifierAbsorbDamageAndDoNotRemainAfterTheBattle(t *testing.T) {
	state := withBattlers(t, invade(t, invadingMatch(t)), 30, 20, 5, 40)
	state = withCreatureEffects(t, state, nil, []effect.Effect{effecttest.HitPointsModifier(20)})

	state, _ = defend(t, state)

	// HP 40 + 20 に 30 のダメージ。増えた 20 を超えた 10 だけが傷になる
	require.Equal(t, creaturetest.HitPoints(t, 30), state.PlacedCreatures[0].HitPoints)
}

func TestDecide_FailedInvasionThatCannotPayTheTollDepletesTheMagic(t *testing.T) {
	state := withMagic(invadingMatch(t), 1, 10)
	state = invade(t, state)

	state, events := defend(t, state)

	require.Equal(t, 1, countOf[*match.MagicDepletedEvent](events))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
	require.Equal(t, 600, state.Participants[1].Magic)
}

// golden の戦闘の入力。項目がすべて出るように、両方にアイテム（守る側は効果をすべて持つ巻物）、守る側に効果、両方に隣の領地を持たせる
func goldenBattleInput(t *testing.T) match.BattleInput {
	t.Helper()
	input := battleInput(t, 30, 40, 20, 35)
	input.Attacker.Item = battleItem(t, 301, card.ItemCategoryWeapon, effect.NewAttackModifierEffect(effect.NewAttackModifier(20)))
	input.Defender.Item = battleItem(t, 302, card.ItemCategoryScroll, effecttest.AttackOverride(t, 30), effect.NewIgnoreTerrainEffect(), effecttest.AttackMultiplier(t, 150, "fire"))
	input.Defender.Spec.Effects = effecttest.Effects(t, effecttest.HitPointsModifier(20))
	input.Attacker.AdjacentTerritories = []board.Position{boardtest.Position(t, 0, 0), boardtest.Position(t, 2, 0)}
	input.Defender.AdjacentTerritories = []board.Position{boardtest.Position(t, 3, 1)}
	return input
}

// 戦闘の入力（攻める側はカード 101・守る側は 201 の性能の AT と戦闘の前の HP を変えたもの。アイテム・隣の領地は無し、土地は火のレベル 1）
func battleInput(t *testing.T, attack, hitPoints, defenderAttack, defenderHitPoints int) match.BattleInput {
	t.Helper()
	combatant := func(id int64, attack, hitPoints int) match.BattleCombatant {
		spec := creatureSpec(t, id)
		spec.Attack = creaturetest.Attack(t, attack)
		return match.BattleCombatant{Card: cardtest.CreatureRef(t, id), Spec: *spec, Attack: spec.Attack, HitPoints: creaturetest.HitPoints(t, hitPoints), AdjacentTerritories: []board.Position{}}
	}
	rules := matchtest.Rules(t, 1, 6, 5)
	land := match.BattleLand{Element: element.LandFire, Level: 1}
	battleRules := match.BattleRules{AdjacentTerritoryBonus: rules.AdjacentTerritoryBonus, TerrainBonusPerLevel: rules.TerrainBonusPerLevel}
	return match.BattleInput{
		Attacker: combatant(101, attack, hitPoints),
		Defender: combatant(201, defenderAttack, defenderHitPoints),
		Land:     land,
		Rules:    battleRules,
	}
}

// 戦闘に出したアイテム（カードの ID と分類と効果）
func battleItem(t *testing.T, id int64, category card.ItemCategory, effects ...effect.Effect) *match.BattleItem {
	t.Helper()
	spec := match.ItemSpec{Category: category, Effects: effecttest.Effects(t, effects...)}
	return &match.BattleItem{Card: cardtest.Ref(t, card.ItemEntityType, id), Spec: spec}
}

// 手札の最後に、性能を持つアイテムを足し、その手札の番号を返す
func withBattleItem(t *testing.T, state match.Match, participant int, id int64, category card.ItemCategory, effects ...effect.Effect) (match.Match, int) {
	t.Helper()
	state = withItemInHand(t, state, participant, id, 0)
	ref := cardtest.Ref(t, card.ItemEntityType, id)
	cards := slices.Clone(state.Participants[participant].Participant.Cards)
	for i := range cards {
		if cards[i].Card == ref {
			cards[i].Spec.Item = &match.ItemSpec{Category: category, Effects: effecttest.Effects(t, effects...)}
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
	state.PlacedCreatures = slices.Clone(state.PlacedCreatures)
	state.PlacedCreatures[0].Base.Elements = creaturetest.Elements(t, elements...)
	return state
}

func TestDecide_BattleInputRecordsNoAdjacentTerritoriesAsAnEmptyList(t *testing.T) {
	_, events := defend(t, invade(t, invadingMatch(t)))

	started := eventOf[*match.BattleStartedEvent](t, events)
	payload, err := json.Marshal(started)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"adjacent_territories":[]`)
}

// 戦闘の状態は層に分けて持つ。増減は出どころごとに、受けたダメージは別に残り、今の AT・HP は層から計算する（core#175）
func TestEvolve_BattleCreatureKeepsEachLayer(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 2, 2)
	state = withTerritory(t, state, 1, 1, 1)
	state = withTerritory(t, state, 1, 3, 1)
	state, _ = drawAndRoll(t, state, 1, 2)
	state, weapon := withBattleItem(t, attacking(t, state), 1, 301, card.ItemCategoryWeapon, effect.NewAttackModifierEffect(effect.NewAttackModifier(25)))
	state = attackWithItem(t, state, weapon)
	state = withDefenderElements(t, withBattlers(t, state, 10, 40, 20, 40), "fire")
	state, armor := withBattleItem(t, state, 0, 302, card.ItemCategoryArmor, effect.NewHitPointsModifierEffect(effect.NewHitPointsModifier(15)))
	events, err := match.Decide(state, &match.DefendCommand{By: matchtest.Index(t, 0), ItemHandIndex: &armor}, startedAt, matchtest.NewRandom())
	require.NoError(t, err)

	var next match.State = state
	var battle match.Battle
	for _, event := range events {
		next, err = match.Evolve(next, event)
		require.NoError(t, err)
		if current := next.(match.Match).Battle; current != nil {
			battle = *current
		}
	}

	attacker, defender := battle.Attacker, battle.Defender
	support := match.BattleModifierSource{Kind: match.BattleModifierSourceSupportEffect}
	terrain := match.BattleModifierSource{Kind: match.BattleModifierSourceTerrainEffect}
	weaponEffect := match.BattleModifierSource{Kind: match.BattleModifierSourceItemEffect, Effect: effect.KindAttackModifier}
	armorEffect := match.BattleModifierSource{Kind: match.BattleModifierSourceItemEffect, Effect: effect.KindHitPointsModifier}
	require.Equal(t, []match.BattleStatModifier{{Source: support, AttackDelta: 20}, {Source: weaponEffect, AttackDelta: 25}}, attacker.Modifiers)
	require.Equal(t, []match.BattleStatModifier{{Source: terrain, HitPointsDelta: 20}, {Source: armorEffect, HitPointsDelta: 15}}, defender.Modifiers)
	// 支援効果 +20・武器 +25 の AT 55 で、地形効果 +20・防具 +15 の HP 75 を攻撃した。守る側の反撃 20 を攻める側が受けた
	require.Equal(t, creaturetest.Attack(t, 20), defender.Base.Attack)
	require.Equal(t, creaturetest.HitPoints(t, 40), defender.InitialHitPoints)
	require.Equal(t, 55, defender.Damage)
	require.Equal(t, match.BattleStats{Attack: creaturetest.Attack(t, 20), HitPoints: creaturetest.HitPoints(t, 20)}, defender.Stats())
	require.Equal(t, 20, attacker.Damage)
	require.Equal(t, match.BattleStats{Attack: creaturetest.Attack(t, 55), HitPoints: creaturetest.HitPoints(t, 20)}, attacker.Stats())
}

// 戦闘の入力は、戦闘が始まる時点の状態から集める（決まりそのものは TestResolveBattle_ProducesTheEventsFromTheInput）
func TestDecide_BattleStartsWithTheInputGatheredFromTheState(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 2, 2)
	state = withTerritory(t, state, 1, 1, 1)
	state = withTerritory(t, state, 1, 3, 1)
	state, _ = drawAndRoll(t, state, 1, 2)
	state, weapon := withBattleItem(t, attacking(t, state), 1, 301, card.ItemCategoryWeapon, effect.NewAttackModifierEffect(effect.NewAttackModifier(25)))
	state = attackWithItem(t, state, weapon)
	state = withDefenderElements(t, withBattlers(t, state, 10, 40, 20, 40), "fire")
	state, armor := withBattleItem(t, state, 0, 302, card.ItemCategoryArmor, effect.NewHitPointsModifierEffect(effect.NewHitPointsModifier(15)))

	state, events := defendWithItem(t, state, armor)

	started, ok := events[1].(*match.BattleStartedEvent)
	require.True(t, ok)
	input := started.Input
	require.Equal(t, creaturetest.Attack(t, 10), input.Attacker.Spec.Attack)
	require.Equal(t, creaturetest.HitPoints(t, 40), input.Attacker.HitPoints)
	require.Equal(t, creaturetest.Attack(t, 20), input.Defender.Spec.Attack)
	require.Equal(t, creaturetest.HitPoints(t, 40), input.Defender.HitPoints)
	require.Equal(t, battleItem(t, 301, card.ItemCategoryWeapon, effect.NewAttackModifierEffect(effect.NewAttackModifier(25))), input.Attacker.Item)
	require.Equal(t, battleItem(t, 302, card.ItemCategoryArmor, effect.NewHitPointsModifierEffect(effect.NewHitPointsModifier(15))), input.Defender.Item)
	require.Equal(t, match.BattleLand{Element: element.LandFire, Level: 2}, input.Land)
	require.Equal(t, []board.Position{pos(t, 1, 0), pos(t, 3, 0)}, input.Attacker.AdjacentTerritories)
	require.Equal(t, []board.Position{}, input.Defender.AdjacentTerritories)
	require.Equal(t, 10, input.Rules.AdjacentTerritoryBonus.Int())
	require.Equal(t, 10, input.Rules.TerrainBonusPerLevel.Int())
	// 支援効果 +20・武器 +25 の AT 55 で、地形効果 +20・防具 +15 の HP 75 を攻撃し、55 のダメージのうち、増えた 35 を超えた 20 だけが傷になる
	repelled := eventOf[*match.InvasionFailedEvent](t, events)
	require.Equal(t, creaturetest.HitPoints(t, 20), repelled.TerritoryHitPoints)
	require.Equal(t, creaturetest.HitPoints(t, 20), state.PlacedCreatures[0].HitPoints)
	require.Nil(t, state.Battle)
}

// 地形効果を無視されると、増えた分で傷を防げず、受けたダメージがそのまま領地に残る
func TestDecide_IgnoredTerrainEffectDoesNotAbsorbTheDamageOfTheTerritory(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 2, 2)
	state, _ = drawAndRoll(t, state, 1, 2)
	state, scroll := withBattleItem(t, attacking(t, state), 1, 301, card.ItemCategoryScroll, effecttest.AttackOverride(t, 30), effect.NewIgnoreTerrainEffect())
	state = attackWithItem(t, state, scroll)
	state = withDefenderElements(t, withBattlers(t, state, 10, 40, 20, 40), "fire")

	state, events := defend(t, state)

	// 地形効果 +20 は無視され、HP 40 に 30 のダメージ（無視されなければ、増えた 20 を超えた 10 だけが傷になり 30 が残る）
	require.Equal(t, 1, countOf[*match.BattleTerrainEffectActivatedEvent](events))
	repelled := eventOf[*match.InvasionFailedEvent](t, events)
	require.Equal(t, creaturetest.HitPoints(t, 10), repelled.TerritoryHitPoints)
	require.Equal(t, creaturetest.HitPoints(t, 10), state.PlacedCreatures[0].HitPoints)
}

// 支援効果は守る側にも付く。どちらの側も、戦場の隣（ボードのつながりだけ）にあるその側の領地を数える（2026-11-15 オーナー）
func TestDecide_AdjacentTerritoriesCountOnlyConnectedTerritoriesOfEachSide(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 2, 1)
	state = withTerritory(t, state, 0, 1, 1)
	state = withTerritory(t, state, 1, 3, 1)
	state = withTerritory(t, state, 1, 5, 1)
	state.Board.Pairs = []board.Pair{boardtest.Pair(t, 2, 0, 5, 0)}
	state, _ = drawAndRoll(t, state, 1, 2)
	state = invade(t, state)

	_, events := defend(t, state)

	input := eventOf[*match.BattleStartedEvent](t, events).Input
	require.Equal(t, []board.Position{pos(t, 3, 0)}, input.Attacker.AdjacentTerritories)
	require.Equal(t, []board.Position{pos(t, 1, 0)}, input.Defender.AdjacentTerritories)
}

func TestDecide_HitPointsAddedByEffectsAreReducedFirstAndTheRestDisappears(t *testing.T) {
	cases := []struct {
		name   string
		attack int
		want   int
	}{
		{name: "damage_within_the_increase", attack: 10, want: 40},
		{name: "damage_beyond_the_increase", attack: 30, want: 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 2)
			state, _ = drawAndRoll(t, state, 1, 1)
			state = withDefenderElements(t, withBattlers(t, invade(t, state), tc.attack, 20, 5, 40), "fire")

			state, _ = defend(t, state)

			require.Equal(t, creaturetest.HitPoints(t, tc.want), state.PlacedCreatures[0].HitPoints)
		})
	}
}

func TestDecide_HitPointsLoweredByAnItemComeBackAfterTheBattle(t *testing.T) {
	state := withBattlers(t, invade(t, invadingMatch(t)), 5, 20, 5, 40)
	state, cursed := withBattleItem(t, state, 0, 302, card.ItemCategoryArmor, effect.NewHitPointsModifierEffect(effect.NewHitPointsModifier(-10)))

	state, _ = defendWithItem(t, state, cursed)

	// HP 40 - 10 に 5 のダメージ。下がった 10 は戦闘の後で戻る
	require.Equal(t, creaturetest.HitPoints(t, 35), state.PlacedCreatures[0].HitPoints)
}

func TestDecide_AttackerDestroyedByAnEffectGoesToTheDiscardPileAndPaysTheToll(t *testing.T) {
	state := withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1)
	state, _ = drawAndRoll(t, state, 1, 1)
	state, cursed := withBattleItem(t, attacking(t, state), 1, 301, card.ItemCategoryArmor, effect.NewHitPointsModifierEffect(effect.NewHitPointsModifier(-99)))
	state = attackWithItem(t, state, cursed)
	state = withDefenderElements(t, withBattlers(t, state, 50, 20, 5, 30), "fire")
	handBefore := len(state.Participants[1].Hand)

	state, events := defend(t, state)

	repelled := eventOf[*match.InvasionFailedEvent](t, events)
	require.Equal(t, creaturetest.HitPoints(t, 30), repelled.TerritoryHitPoints)
	require.Len(t, state.Participants[1].Hand, handBefore)
	require.Equal(t, 1, countOf[*match.TollPaidEvent](events))
}

func TestDecide_DefenderDestroyedByAnEffectIsConquered(t *testing.T) {
	state := withBattlers(t, invade(t, invadingMatch(t)), 5, 20, 50, 30)
	state, cursed := withBattleItem(t, state, 0, 302, card.ItemCategoryArmor, effect.NewHitPointsModifierEffect(effect.NewHitPointsModifier(-99)))

	state, events := defendWithItem(t, state, cursed)

	conquered := eventOf[*match.InvasionSucceededEvent](t, events)
	require.Equal(t, creaturetest.HitPoints(t, 20), conquered.TerritoryHitPoints)
	require.Equal(t, matchtest.Index(t, 1), state.PlacedCreatures[0].Owner)
	require.Equal(t, creaturetest.HitPoints(t, 20), state.PlacedCreatures[0].HitPoints)
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
	return apply(t, state, matchtest.NewRandom(), &match.LevelUpLandCommand{By: matchtest.Index(t, 1), Position: pos(t, x, 0), Level: level})
}

func TestDecide_PassingOwnTerritoryOffersLevelUpsWithinTheMagic(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 2)

	require.Equal(t, match.PhasePlacing, state.Phase)
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

	require.Equal(t, match.PhasePlacing, state.Phase)
	require.Empty(t, state.LevelUpChoices())
}

func TestDecide_TerritoryWhereTheTurnStoppedIsOffered(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 1)

	require.Equal(t, match.PhaseTerritoryCommanding, state.Phase)
	require.Equal(t, pos(t, 1, 0), state.LevelUpChoices()[0].Position)
}

func TestDecide_TerritoriesOfOthersAreNotOffered(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 0, 1, 1), 2)

	require.Equal(t, match.PhasePlacing, state.Phase)
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
		{name: "vacant_land", setup: func(_ *testing.T, state match.Match) match.Match { return state }, face: 2, want: match.PhasePlacing},
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

// 拠点 (0,0)・土地 (1,0)・(2,0)・チェックポイント (3,0)・土地 (4,0)・(5,0) の一本道
func checkpointBoard(t *testing.T) match.Board {
	t.Helper()
	tiles := []board.Tile{boardtest.Tile(t, 0, 0, "base", "")}
	var connections []board.Connection
	for i, kind := range []string{"land", "land", "checkpoint", "land", "land"} {
		element := ""
		if kind == "land" {
			element = "fire"
		}
		tiles = append(tiles, boardtest.Tile(t, i+1, 0, kind, element))
		connections = append(connections, boardtest.Connection(t, i, 0, i+1, 0))
	}
	return match.Board{ID: boardtest.ID(t, 6), Size: boardtest.Size(t, 6, 1), Tiles: tiles, Connections: connections}
}

func levelUpPositions(state match.Match) []board.Position {
	var positions []board.Position
	for _, choice := range state.LevelUpChoices() {
		if !slices.Contains(positions, choice.Position) {
			positions = append(positions, choice.Position)
		}
	}
	return positions
}

func TestDecide_StoppingExactlyOnABaseOrACheckpointOffersAllOwnTerritories(t *testing.T) {
	cases := []struct {
		name  string
		start int
		face  int
	}{
		{name: "checkpoint", start: 0, face: 3},
		{name: "base", start: 2, face: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := newMatch(t, checkpointBoard(t), defaultRules(t))
			state = withTerritory(t, withTerritory(t, withTerritory(t, state, 1, 5, 1), 0, 4, 1), 1, 1, 1)
			state.Participants = slices.Clone(state.Participants)
			state.Participants[1].Position = pos(t, tc.start, 0)
			previous := pos(t, tc.start+1, 0)
			if tc.start == 0 {
				state.Participants[1].Previous = nil
			} else {
				state.Participants[1].Previous = &previous
			}

			state = rolledTo(t, state, tc.face)

			require.Equal(t, match.PhaseTerritoryCommanding, state.Phase)
			// 通っていない (5,0) も入り、他人の (4,0) は入らない（並びは領地ができた順）
			require.Equal(t, []board.Position{pos(t, 5, 0), pos(t, 1, 0)}, levelUpPositions(state))
		})
	}
}

func TestDecide_PassingACheckpointOffersOnlyPassedTerritories(t *testing.T) {
	state := newMatch(t, checkpointBoard(t), defaultRules(t))
	state = withTerritory(t, withTerritory(t, state, 1, 5, 1), 1, 1, 1)

	state = rolledTo(t, state, 4)

	require.Equal(t, match.PhasePlacing, state.Phase)
	require.Equal(t, []board.Position{pos(t, 1, 0)}, levelUpPositions(state))
}

func TestDecide_LevelUpOnATerritoryNotPassedAfterStoppingOnACheckpoint(t *testing.T) {
	state := withTerritory(t, newMatch(t, checkpointBoard(t), defaultRules(t)), 1, 5, 1)
	state = rolledTo(t, state, 3)

	state, events := levelUp(t, state, 5, 2)

	require.Equal(t, 1, countOf[*match.LandLeveledUpEvent](events))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
	require.Equal(t, 2, state.LandLevel(pos(t, 5, 0)))
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

	leveled := eventOf[*match.LandLeveledUpEvent](t, events)
	require.Equal(t, 3, leveled.Level)
	require.Equal(t, 300, leveled.Magic)
	require.Equal(t, 600-300, state.Participants[1].Magic)
	require.Equal(t, 400, valueAt(t, state, 1))
	require.Zero(t, countOf[*match.CreaturePlacedEvent](events))
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
			err := decideErr(t, state, &match.LevelUpLandCommand{By: matchtest.Index(t, 1), Position: pos(t, tc.x, 0), Level: tc.level})

			require.ErrorIs(t, err, match.ErrTerritoryNotLevelUpable)
		})
	}
}

// 通った領地にいるクリーチャーを、手札に無いカードにして傷を付ける（手札に戻るカードを見分けるため）
func withWoundedRare(t *testing.T, state match.Match) match.Match {
	t.Helper()
	state.PlacedCreatures = slices.Clone(state.PlacedCreatures)
	state.PlacedCreatures[0].Card = cardtest.CreatureRef(t, 999)
	state.PlacedCreatures[0].HitPoints = creaturetest.HitPoints(t, 1)
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
	replaced := state.PlacedCreatures[0].ID

	state, events := exchange(t, state, 1, 0)

	exchanged := eventOf[*match.CreatureExchangedEvent](t, events)
	require.Equal(t, cardtest.CreatureRef(t, 201), state.PlacedCreatures[len(state.PlacedCreatures)-1].Card)
	require.NotEqual(t, replaced, exchanged.Creature)
	require.Equal(t, 600-10, state.Participants[1].Magic)
	require.Len(t, state.Participants[1].Hand, handBefore)
	require.Equal(t, cardtest.CreatureRef(t, 999), state.Participants[1].Hand[handBefore-1])
	require.Len(t, state.PlacedCreatures, 1)
	require.Equal(t, exchanged.Creature, state.PlacedCreatures[0].ID)
	require.Equal(t, creaturetest.HitPoints(t, 20), state.PlacedCreatures[0].HitPoints)
	require.Equal(t, matchtest.Index(t, 1), state.PlacedCreatures[0].Owner)
	require.Equal(t, []match.Territory{{Position: pos(t, 1, 0), Creature: exchanged.Creature}}, state.Territories)
	require.Equal(t, state.LastPlacedCreatureID, exchanged.Creature)
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

func changeElement(t *testing.T, state match.Match, x int, element element.Land) (match.Match, []match.Event) {
	t.Helper()
	return apply(t, state, matchtest.NewRandom(), &match.ChangeLandElementCommand{By: matchtest.Index(t, 1), Position: pos(t, x, 0), Element: element})
}

func TestMatch_ElementChangeChoicesAreOtherElementsWithinTheMagic(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 2)

	require.Equal(t, []match.ElementChangeChoice{
		{Position: pos(t, 1, 0), Element: element.LandEarth, Magic: 200, Multiplier: 2},
		{Position: pos(t, 1, 0), Element: element.LandWater, Magic: 200, Multiplier: 2},
		{Position: pos(t, 1, 0), Element: element.LandWind, Magic: 200, Multiplier: 2},
	}, state.ElementChangeChoices())
	require.Empty(t, withMagic(state, 1, 199).ElementChangeChoices())
}

func TestMatch_ChangingFromMultiLandIsCheaperAndOffersAllFourElements(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 5, 1), 5)

	choices := state.ElementChangeChoices()

	require.Len(t, choices, 4)
	for _, choice := range choices {
		require.Equal(t, 100, choice.Magic)
		require.Equal(t, 1, choice.Multiplier)
	}
}

func TestDecide_ChangingTheElementRecountsTheChainAndEndsTheTurn(t *testing.T) {
	state := withTerritory(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 2), 1, 2, 1)
	state = rolledTo(t, state, 3)
	require.Equal(t, 150, valueAt(t, state, 2))

	state, events := changeElement(t, state, 1, element.LandWater)

	changed := eventOf[*match.LandElementChangedEvent](t, events)
	require.Equal(t, element.LandWater, changed.Element)
	require.Equal(t, 200, changed.Magic)
	require.Equal(t, 600-200, state.Participants[1].Magic)
	require.Equal(t, 100, valueAt(t, state, 2))
	// レベルとクリーチャーはそのまま
	require.Equal(t, 2, state.LandLevel(pos(t, 1, 0)))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

func TestDecide_ChangingFromMultiLandPaysTheCheaperMultiplier(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 5, 1), 5)

	choice := state.ElementChangeChoices()[0]

	_, events := apply(t, state, matchtest.NewRandom(), &match.ChangeLandElementCommand{By: matchtest.Index(t, 1), Position: choice.Position, Element: choice.Element})

	// 基本の値 100 × 複属性の土地からの倍率 1
	require.Equal(t, 100, eventOf[*match.LandElementChangedEvent](t, events).Magic)
}

func TestDecide_ChangingTheElementOnTheTerritoryOfAnotherOnlyPaysTheToll(t *testing.T) {
	state := withTerritory(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 0, 2, 1)
	state = rolledTo(t, state, 2)

	_, events := changeElement(t, state, 1, element.LandWater)

	require.Equal(t, 1, countOf[*match.TollPaidEvent](events))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

func TestDecide_ElementChangeRejectsChoicesThatAreNotOffered(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 2)
	cases := []struct {
		name    string
		x       int
		element element.Land
	}{
		{name: "same_element", x: 1, element: element.LandFire},
		{name: "to_multi", x: 1, element: element.LandMulti},
		{name: "to_neutral", x: 1, element: element.LandNeutral},
		{name: "not_passed", x: 3, element: element.LandWater},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := decideErr(t, state, &match.ChangeLandElementCommand{By: matchtest.Index(t, 1), Position: pos(t, tc.x, 0), Element: tc.element})

			require.ErrorIs(t, err, match.ErrLandElementNotChangeable)
		})
	}
}

// その位置の領地のクリーチャーの性能を変える
func withCreatureAt(t *testing.T, state match.Match, x, attack, hitPoints int) match.Match {
	t.Helper()
	territory, ok := state.TerritoryAt(pos(t, x, 0))
	require.True(t, ok)
	state.PlacedCreatures = slices.Clone(state.PlacedCreatures)
	for i := range state.PlacedCreatures {
		if state.PlacedCreatures[i].ID == territory.Creature {
			state.PlacedCreatures[i].Base.Attack = creaturetest.Attack(t, attack)
			state.PlacedCreatures[i].Attack = state.PlacedCreatures[i].Base.Attack
			state.PlacedCreatures[i].HitPoints = creaturetest.HitPoints(t, hitPoints)
		}
	}
	return state
}

func move(t *testing.T, state match.Match, from, to int) (match.Match, []match.Event) {
	t.Helper()
	return apply(t, state, matchtest.NewRandom(), &match.MoveCreatureCommand{By: matchtest.Index(t, 1), From: pos(t, from, 0), To: pos(t, to, 0)})
}

func TestMatch_MoveChoicesAreOneStepAlongTheConnections(t *testing.T) {
	state := rolledTo(t, withCreatureAt(t, withTerritory(t, territoryBoardMatch(t), 1, 3, 1), 3, 5, 20), 3)

	require.Equal(t, []match.MoveChoice{{From: pos(t, 3, 0), To: pos(t, 2, 0)}, {From: pos(t, 3, 0), To: pos(t, 4, 0)}}, state.MoveChoices())
}

// 防御型（侵略できない・移動できない）
func defensiveEffects(t *testing.T) effect.Effects {
	t.Helper()
	return effecttest.Effects(t, effect.NewForbidInvasionEffect(), effect.NewForbidMovementEffect())
}

func TestMatch_DefensiveCreaturesCannotMove(t *testing.T) {
	state := rolledTo(t, withCreatureAt(t, withTerritory(t, territoryBoardMatch(t), 1, 3, 1), 3, 5, 20), 3)
	state.PlacedCreatures = slices.Clone(state.PlacedCreatures)
	state.PlacedCreatures[0].Base.Effects = defensiveEffects(t)

	require.Empty(t, state.MoveChoices())
}

// 手札のクリーチャーをすべて防御型にする（性能は写しから読むため）
func withHandDefensive(t *testing.T, state match.Match, participant int) match.Match {
	t.Helper()
	state.Participants = slices.Clone(state.Participants)
	target := state.Participants[participant]
	target.Participant.Cards = slices.Clone(target.Participant.Cards)
	for i, deckCard := range target.Participant.Cards {
		if deckCard.Spec.Creature != nil {
			defensive := *deckCard.Spec.Creature
			defensive.Effects = defensiveEffects(t)
			target.Participant.Cards[i].Spec.Creature = &defensive
		}
	}
	state.Participants[participant] = target
	return state
}

func TestMatch_DefensiveCreaturesCannotInvadeButCanBePlaced(t *testing.T) {
	invading := invadingMatch(t)
	require.NotEmpty(t, invading.InvadeChoices())
	require.Empty(t, withHandDefensive(t, invading, 1).InvadeChoices())

	placing := rolledTo(t, territoryBoardMatch(t), 2)
	require.NotEmpty(t, withHandDefensive(t, placing, 1).PlaceChoices())
}

func TestDecide_InvadingWithADefensiveCreatureIsRejected(t *testing.T) {
	state := withHandDefensive(t, invadingMatch(t), 1)

	require.ErrorIs(t, decideErr(t, state, &match.InvadeCommand{By: matchtest.Index(t, 1), HandIndex: 0}), match.ErrCreatureNotInvadable)
}

func TestMatch_MoveChoicesExcludeOwnTerritoriesAndTilesThatAreNotLand(t *testing.T) {
	state := withTerritory(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 1, 2, 1)
	state = rolledTo(t, state, 2)

	require.Equal(t, []match.MoveChoice{{From: pos(t, 2, 0), To: pos(t, 3, 0)}}, state.MoveChoices())
}

func TestDecide_MovingToVacantLandKeepsTheCreatureAndLeavesTheLevelsOnTheLands(t *testing.T) {
	state := withTerritory(t, territoryBoardMatch(t), 1, 1, 3)
	state.LandLevels = append(slices.Clone(state.LandLevels), match.LandLevel{Position: pos(t, 2, 0), Level: 2})
	state = withCreatureAt(t, state, 1, 5, 7)
	mover := state.PlacedCreatures[0].ID
	state = rolledTo(t, state, 1)

	state, events := move(t, state, 1, 2)

	require.Equal(t, 1, countOf[*match.CreatureMovedEvent](events))
	require.Equal(t, []match.Territory{{Position: pos(t, 2, 0), Creature: mover}}, state.Territories)
	require.Equal(t, creaturetest.HitPoints(t, 7), state.PlacedCreatures[0].HitPoints)
	require.Equal(t, 3, state.LandLevel(pos(t, 1, 0)))
	require.Equal(t, 2, state.LandLevel(pos(t, 2, 0)))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

// 自分（参加者 1）の領地 (1,0) から、参加者 0 の領地 (2,0) へ動いて戦闘する
func movingIntoAnother(t *testing.T, moverAttack, moverHitPoints, defenderAttack, defenderHitPoints int) (match.Match, []match.Event) {
	t.Helper()
	state := withTerritory(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 0, 2, 2)
	state = withCreatureAt(t, state, 1, moverAttack, moverHitPoints)
	state = withCreatureAt(t, state, 2, defenderAttack, defenderHitPoints)
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

	conquered := eventOf[*match.InvasionSucceededEvent](t, events)
	mover := state.PlacedCreatures[0].ID
	require.Equal(t, mover, conquered.Creature)
	require.Equal(t, []match.Territory{{Position: pos(t, 2, 0), Creature: mover}}, state.Territories)
	require.Len(t, state.PlacedCreatures, 1)
	require.Equal(t, matchtest.Index(t, 1), state.PlacedCreatures[0].Owner)
	require.Equal(t, 2, state.LandLevel(pos(t, 2, 0)))
	require.Zero(t, countOf[*match.TollPaidEvent](events))
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

func TestDecide_MovingIntoAnotherAndSurvivingReturnsToTheOriginWithTheWound(t *testing.T) {
	state, events := movingIntoAnother(t, 5, 20, 5, 20)

	repelled := eventOf[*match.InvasionFailedEvent](t, events)
	require.Equal(t, creaturetest.HitPoints(t, 15), *repelled.ReturnedHitPoints)
	territory, ok := state.TerritoryAt(pos(t, 1, 0))
	require.True(t, ok)
	mover, _ := state.PlacedCreature(territory.Creature)
	require.Equal(t, creaturetest.HitPoints(t, 15), mover.HitPoints)
	require.Nil(t, state.Invasion)
	require.Equal(t, 1, countOf[*match.TurnEndedEvent](events))
}

func TestDecide_MovingIntoAnotherAndBeingDestroyedFreesTheOrigin(t *testing.T) {
	state, events := movingIntoAnother(t, 5, 5, 10, 20)

	repelled := eventOf[*match.InvasionFailedEvent](t, events)
	requireDestroyedBefore[*match.InvadingCreatureDestroyedEvent, *match.InvasionFailedEvent](t, events)
	require.Nil(t, repelled.ReturnedHitPoints)
	_, occupied := state.TerritoryAt(pos(t, 1, 0))
	require.False(t, occupied)
	require.Len(t, state.PlacedCreatures, 1)
	require.Len(t, state.Participants[1].DiscardPile, 1)
}

func TestDecide_AdjacentTerritoriesOfAMoveDoNotCountTheOrigin(t *testing.T) {
	state := withTerritory(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 0, 2, 1)
	state = withTerritory(t, state, 1, 3, 1)
	state = withCreatureAt(t, state, 1, 5, 20)
	state = rolledTo(t, state, 1)
	state, _ = move(t, state, 1, 2)
	state, _ = apply(t, state, matchtest.NewRandom(), &match.AttackCommand{By: matchtest.Index(t, 1)})

	_, events := defend(t, state)

	require.Equal(t, []board.Position{pos(t, 3, 0)}, eventOf[*match.BattleStartedEvent](t, events).Input.Attacker.AdjacentTerritories)
}

func TestDecide_MoveRejectsChoicesThatAreNotOffered(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 2)

	err := decideErr(t, state, &match.MoveCreatureCommand{By: matchtest.Index(t, 1), From: pos(t, 1, 0), To: pos(t, 3, 0)})

	require.ErrorIs(t, err, match.ErrCreatureNotMovable)
}

func TestDecide_TerritoryCommandsAreRejectedOutsideTheirPhase(t *testing.T) {
	state := territoryBoardMatch(t)
	by := matchtest.Index(t, 1)

	require.ErrorIs(t, decideErr(t, state, &match.LevelUpLandCommand{By: by, Position: pos(t, 1, 0), Level: 2}), match.ErrPhaseNotAllowed)
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

// 手札の最後のカードを、分類を持つアイテムにする
func withLastItemCategory(t *testing.T, state match.Match, participant int, category card.ItemCategory) match.Match {
	t.Helper()
	state.Participants = slices.Clone(state.Participants)
	target := state.Participants[participant]
	target.Participant.Cards = slices.Clone(target.Participant.Cards)
	last := len(target.Participant.Cards) - 1
	target.Participant.Cards[last].Spec.Item = &match.ItemSpec{Category: category}
	state.Participants[participant] = target
	return state
}

func TestMatch_ItemChoicesExcludeTheCategoriesTheFighterCannotHold(t *testing.T) {
	restrictions := creaturetest.ItemRestrictions(t, "armor")
	attackerState := func(t *testing.T, category card.ItemCategory) match.Match {
		t.Helper()
		state := withLastItemCategory(t, withItemInHand(t, attacking(t, invadingMatch(t)), 1, 301, 20), 1, category)
		invasion := *state.Invasion
		invasion.Base.ItemRestrictions = restrictions
		state.Invasion = &invasion
		return state
	}
	defenderState := func(t *testing.T, category card.ItemCategory) match.Match {
		t.Helper()
		state := withLastItemCategory(t, withItemInHand(t, invade(t, invadingMatch(t)), 0, 301, 20), 0, category)
		state.PlacedCreatures = slices.Clone(state.PlacedCreatures)
		for i := range state.PlacedCreatures {
			state.PlacedCreatures[i].Base.ItemRestrictions = restrictions
		}
		return state
	}
	tests := []struct {
		name  string
		state func(*testing.T, card.ItemCategory) match.Match
		by    int
	}{
		{name: "attacker", state: attackerState, by: 1},
		{name: "defender", state: defenderState, by: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed := tt.state(t, card.ItemCategoryWeapon)
			require.Equal(t, []int{len(allowed.Participants[tt.by].Hand) - 1}, allowed.ItemChoices())
			require.Empty(t, tt.state(t, card.ItemCategoryArmor).ItemChoices())
		})
	}
}

func TestDecide_AttackWithAnItemTheFighterCannotHoldIsRejected(t *testing.T) {
	state := withLastItemCategory(t, withItemInHand(t, attacking(t, invadingMatch(t)), 1, 301, 20), 1, card.ItemCategoryScroll)
	invasion := *state.Invasion
	invasion.Base.ItemRestrictions = creaturetest.ItemRestrictions(t, "scroll")
	state.Invasion = &invasion
	itemIndex := len(state.Participants[1].Hand) - 1

	require.ErrorIs(t, decideErr(t, state, &match.AttackCommand{By: matchtest.Index(t, 1), ItemHandIndex: &itemIndex}), match.ErrItemNotUsable)
}

// 参加者の山札の全部のクリーチャーの性能に配置制限を入れる（性能は写しから読むため）
func withHandRestrictedFrom(t *testing.T, state match.Match, participant int, elements ...string) match.Match {
	t.Helper()
	state.Participants = slices.Clone(state.Participants)
	target := state.Participants[participant]
	target.Participant.Cards = slices.Clone(target.Participant.Cards)
	for i, deckCard := range target.Participant.Cards {
		if deckCard.Spec.Creature != nil {
			restricted := *deckCard.Spec.Creature
			restricted.PlacementRestrictions = creaturetest.PlacementRestrictions(t, elements...)
			target.Participant.Cards[i].Spec.Creature = &restricted
		}
	}
	state.Participants[participant] = target
	return state
}

// 配置済みのクリーチャーすべての性能に配置制限を入れる
func withPlacedRestrictedFrom(t *testing.T, state match.Match, elements ...string) match.Match {
	t.Helper()
	state.PlacedCreatures = slices.Clone(state.PlacedCreatures)
	for i := range state.PlacedCreatures {
		state.PlacedCreatures[i].Base.PlacementRestrictions = creaturetest.PlacementRestrictions(t, elements...)
	}
	return state
}

func TestMatch_PlaceChoicesExcludeCreaturesRestrictedFromTheLand(t *testing.T) {
	stoppedOnFire := rolledTo(t, territoryBoardMatch(t), 2)

	require.NotEmpty(t, stoppedOnFire.PlaceChoices())
	require.Empty(t, withHandRestrictedFrom(t, stoppedOnFire, 1, "fire").PlaceChoices())
	require.NotEmpty(t, withHandRestrictedFrom(t, stoppedOnFire, 1, "water").PlaceChoices())
}

func TestMatch_NeutralAndMultiLandsAreNeverRestricted(t *testing.T) {
	tests := []struct {
		name string
		roll int
	}{
		{name: "multi_land", roll: 5},
		{name: "neutral_land", roll: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := withHandRestrictedFrom(t, rolledTo(t, territoryBoardMatch(t), tt.roll), 1, "fire", "earth", "water", "wind")

			require.NotEmpty(t, state.PlaceChoices())
		})
	}
}

func TestMatch_InvadeChoicesExcludeCreaturesRestrictedFromTheLand(t *testing.T) {
	state := invadingMatch(t)

	require.NotEmpty(t, state.InvadeChoices())
	require.Empty(t, withHandRestrictedFrom(t, state, 1, "fire").InvadeChoices())
}

func TestMatch_ExchangeChoicesExcludeCreaturesRestrictedFromTheLand(t *testing.T) {
	state := rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 2)

	require.NotEmpty(t, state.ExchangeChoices())
	require.Empty(t, withHandRestrictedFrom(t, state, 1, "fire").ExchangeChoices())
}

func TestMatch_MoveChoicesExcludeLandsTheCreatureIsRestrictedFrom(t *testing.T) {
	// 3 にいるクリーチャーの 1 歩先は、2（火）と 4（水）
	state := rolledTo(t, withCreatureAt(t, withTerritory(t, territoryBoardMatch(t), 1, 3, 1), 3, 5, 20), 3)
	require.Equal(t, []match.MoveChoice{{From: pos(t, 3, 0), To: pos(t, 2, 0)}, {From: pos(t, 3, 0), To: pos(t, 4, 0)}}, state.MoveChoices())

	restricted := withPlacedRestrictedFrom(t, state, "water")

	require.Equal(t, []match.MoveChoice{{From: pos(t, 3, 0), To: pos(t, 2, 0)}}, restricted.MoveChoices())
}

func TestMatch_ElementChangeChoicesExcludeElementsTheCreatureIsRestrictedFrom(t *testing.T) {
	state := withPlacedRestrictedFrom(t, rolledTo(t, withTerritory(t, territoryBoardMatch(t), 1, 1, 1), 2), "earth", "wind")

	require.Equal(t, []match.ElementChangeChoice{
		{Position: pos(t, 1, 0), Element: element.LandWater, Magic: 200, Multiplier: 2},
	}, state.ElementChangeChoices())
}

func TestDecide_PlaceOnARestrictedLandIsRejected(t *testing.T) {
	state := withHandRestrictedFrom(t, rolledTo(t, territoryBoardMatch(t), 2), 1, "fire")

	require.ErrorIs(t, decideErr(t, state, &match.PlaceCreatureCommand{By: matchtest.Index(t, 1), HandIndex: 0}), match.ErrCreatureNotPlaceable)
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
	started, err := match.Evolve(defending, &match.BattleStartedEvent{EventMeta: meta, Input: battleInput(t, 5, 20, 5, 20)})
	require.NoError(t, err)
	battling := started.(match.Match)
	cost := cardtest.Cost(t, 10)
	damage := creaturetest.Attack(t, 1)
	hitPoints := creaturetest.HitPoints(t, 1)
	defeated := battling.Territories[0].Creature
	next := matchtest.PlacedCreatureID(t, battling.LastPlacedCreatureID.Int()+1)
	cases := []struct {
		name  string
		state match.Match
		event match.Event
	}{
		{name: "invaded_outside_the_phase", state: defending, event: &match.TerritoryInvadedEvent{EventMeta: meta, Participant: by, Cost: cost}},
		{name: "invaded_with_a_hand_index_out_of_range", state: invading, event: &match.TerritoryInvadedEvent{EventMeta: meta, Participant: by, HandIndex: 99, Cost: cost}},
		{name: "item_outside_the_phase", state: invading, event: &match.ItemUsedEvent{EventMeta: meta, Participant: by, Cost: cost}},
		{name: "item_not_in_the_hand", state: armed, event: &match.ItemUsedEvent{EventMeta: meta, Participant: by, HandIndex: 99, Cost: cost}},
		{name: "item_by_who_is_not_awaited", state: defending, event: &match.ItemUsedEvent{EventMeta: meta, Participant: by, Cost: cost}},
		{name: "skipped_outside_the_phase", state: invading, event: &match.ItemSkippedEvent{EventMeta: meta, Participant: by}},
		{name: "skipped_by_who_is_not_awaited", state: defending, event: &match.ItemSkippedEvent{EventMeta: meta, Participant: by}},
		{name: "battle_started_outside_the_phase", state: invading, event: &match.BattleStartedEvent{EventMeta: meta, Input: battleInput(t, 5, 20, 5, 20)}},
		{name: "defending_creature_destroyed_before_the_battle", state: defending, event: &match.DefendingCreatureDestroyedEvent{EventMeta: meta, Participant: by}},
		{name: "invading_creature_destroyed_before_the_battle", state: defending, event: &match.InvadingCreatureDestroyedEvent{EventMeta: meta, Participant: by}},
		{name: "board_creature_destroyed_without_a_creature_at_zero", state: invading, event: &match.BoardCreatureDestroyedEvent{EventMeta: meta, Participant: by}},
		{name: "support_effect_before_the_battle", state: defending, event: &match.BattleSupportEffectActivatedEvent{EventMeta: meta, Side: match.BattleSideAttacker, AttackDelta: 10}},
		{name: "terrain_effect_before_the_battle", state: defending, event: &match.BattleTerrainEffectActivatedEvent{EventMeta: meta, HitPointsDelta: 10}},
		{name: "item_effect_before_the_battle", state: defending, event: &match.BattleItemEffectActivatedEvent{EventMeta: meta, Side: match.BattleSideAttacker, Effect: effect.KindAttackModifier, Change: &match.BattleStatChange{}}},
		{name: "toll_multiplier_without_territory", state: invading, event: &match.TollCreatureEffectActivatedEvent{EventMeta: meta}},
		{name: "creature_effect_before_the_battle", state: defending, event: &match.BattleCreatureEffectActivatedEvent{EventMeta: meta, Side: match.BattleSideAttacker, Effect: effect.KindStrikeFirst}},
		{name: "struck_before_the_battle", state: defending, event: &match.CreatureStruckEvent{EventMeta: meta, Striker: match.BattleSideAttacker, Damage: damage, RemainingHitPoints: hitPoints}},
		{name: "struck_without_invasion", state: invading, event: &match.CreatureStruckEvent{EventMeta: meta, Striker: match.BattleSideAttacker, Damage: damage, RemainingHitPoints: hitPoints}},
		{name: "conquered_before_the_battle", state: defending, event: &match.InvasionSucceededEvent{EventMeta: meta, Participant: by, Creature: next}},
		{name: "conquered_with_a_used_id", state: battling, event: &match.InvasionSucceededEvent{EventMeta: meta, Participant: by, Creature: defeated}},
		{name: "repelled_before_the_battle", state: defending, event: &match.InvasionFailedEvent{EventMeta: meta, Participant: by}},
		{name: "repelled_returning_to_an_origin_without_moving", state: battling, event: &match.InvasionFailedEvent{EventMeta: meta, Participant: by, ReturnedHitPoints: &hitPoints}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := match.Evolve(tc.state, tc.event)

			require.ErrorIs(t, err, match.ErrInvalidState)
		})
	}
}

// 手札の最後にスペルを足す（デッキの写しにも足す。性能は写しから読むため）
func withSpellInHand(t *testing.T, state match.Match, participant int, id int64, cost int, effect effect.SpellEffect) match.Match {
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
	require.Equal(t, []match.SpellChoice{{HandIndex: spellIndex(state), Targets: []match.ParticipantIndex{}, TargetPositions: []board.Position{}}}, state.SpellChoices())

	state, events := apply(t, state, matchtest.NewRandom(), &match.UseSpellCommand{By: matchtest.Index(t, 1), HandIndex: spellIndex(state)})

	used := eventOf[*match.SpellUsedEvent](t, events)
	require.Equal(t, cardtest.Cost(t, 40), used.Cost)
	require.Contains(t, state.Participants[1].DiscardPile, cardtest.Ref(t, card.SpellEntityType, 401))
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
				return withSpellInHand(t, rollingWithSpell(t), 1, 402, 10, effect.NoSpellEffect())
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

			gained := eventOf[*match.MagicGainedEvent](t, events)
			require.Equal(t, tt.want, gained.Magic)
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

func TestDecide_MultipleDiceOnYourselfRollsTheDiceTogetherAndEndsAtTheEndOfTheTurn(t *testing.T) {
	state, events := useSpellOn(t, rollingWithLastingSpell(t, spelltest.MultipleDice(t, 2, 1)), 1)
	require.Equal(t, matchtest.Index(t, 1), eventOf[*match.SpellEffectStartedEvent](t, events).Target)
	require.Equal(t, lasting(spelltest.MultipleDice(t, 2, 1), 1), state.Participants[1].LastingSpellEffect)
	require.Equal(t, match.PhaseRolling, state.Phase)

	state, events = apply(t, state, matchtest.NewRollingRandom(1, 2), &match.RollCommand{By: matchtest.Index(t, 1)})
	rolled := eventOf[*match.DiceRolledEvent](t, events)
	require.Equal(t, []int{1, 2}, rolled.Dice)
	require.Equal(t, 3, rolled.Value)
	require.Equal(t, 3, state.Dice)

	state, events = endTurn(t, state, 1)
	require.Equal(t, 1, countOf[*match.SpellEffectEndedEvent](events))
	require.Nil(t, state.Participants[1].LastingSpellEffect)
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

// 参加者 1 の振る段階に、ターンをまたいで効き続ける効果のスペル 404 を手札に足す
func rollingWithLastingSpell(t *testing.T, effect effect.SpellEffect) match.Match {
	t.Helper()
	return withSpellInHand(t, rollingWithSpell(t), 1, 404, 50, effect)
}

func useSpellOn(t *testing.T, state match.Match, target int) (match.Match, []match.Event) {
	t.Helper()
	targetIndex := matchtest.Index(t, target)
	return apply(t, state, matchtest.NewRandom(), &match.UseSpellCommand{By: matchtest.Index(t, 1), HandIndex: spellIndex(state), Target: &targetIndex})
}

func lasting(effect effect.SpellEffect, remainingTurns int) *match.LastingSpellEffect {
	return &match.LastingSpellEffect{Effect: effect, RemainingTurns: remainingTurns}
}

func TestMatch_SpellChoicesOfALastingEffectOfferEveryParticipant(t *testing.T) {
	tests := map[string]effect.SpellEffect{
		"multiple_dice": spelltest.MultipleDice(t, 2, 1),
		"fixed_dice":    spelltest.FixedDice(t, 6, 1),
	}
	for name, effect := range tests {
		t.Run(name, func(t *testing.T) {
			state := rollingWithLastingSpell(t, effect)

			choices := state.SpellChoices()

			require.Contains(t, choices, match.SpellChoice{HandIndex: spellIndex(state), Targets: indexes(t, 0, 1), TargetPositions: []board.Position{}})
		})
	}
}

func TestDecide_FixedDiceOnYourselfFixesThisTurnsDiceAndEndsAtTheEndOfTheTurn(t *testing.T) {
	state, events := useSpellOn(t, rollingWithLastingSpell(t, spelltest.FixedDice(t, 6, 1)), 1)
	require.Equal(t, matchtest.Index(t, 1), eventOf[*match.SpellEffectStartedEvent](t, events).Target)
	require.Equal(t, lasting(spelltest.FixedDice(t, 6, 1), 1), state.Participants[1].LastingSpellEffect)

	state, events = apply(t, state, matchtest.NewRollingRandom(2), &match.RollCommand{By: matchtest.Index(t, 1)})
	rolled := eventOf[*match.DiceRolledEvent](t, events)
	require.Nil(t, rolled.Dice)
	require.Equal(t, 6, rolled.Value)
	require.Equal(t, 6, state.Dice)

	state, events = endTurn(t, state, 1)
	require.Equal(t, 1, countOf[*match.SpellEffectEndedEvent](events))
	require.Nil(t, state.Participants[1].LastingSpellEffect)
}

func TestDecide_LastingEffectOnAnotherAppliesToTheirNextRoll(t *testing.T) {
	state, _ := useSpellOn(t, rollingWithLastingSpell(t, spelltest.FixedDice(t, 6, 1)), 0)
	state, _ = drawAndRoll(t, state, 1, 2)
	state, events := endTurn(t, state, 1)
	// 使った人のターンの終わりには、対象の人の効果は減らない
	require.Zero(t, countOf[*match.SpellEffectEndedEvent](events))
	require.Equal(t, 1, state.Participants[0].LastingSpellEffect.RemainingTurns)

	state, events = drawAndRoll(t, state, 0, 3)

	require.Equal(t, 6, eventOf[*match.DiceRolledEvent](t, events).Value)
	require.Equal(t, 6, state.Dice)
}

// 1 人に 1 つまでなので、目を固定する効果がかかっていても、複数振る効果を使えば複数振る（2026-11-13 オーナー。core#173）
func TestDecide_NewLastingEffectOverwritesTheOldOne(t *testing.T) {
	state := rollingWithLastingSpell(t, spelltest.MultipleDice(t, 2, 1))
	state.Participants = slices.Clone(state.Participants)
	state.Participants[1].LastingSpellEffect = lasting(spelltest.FixedDice(t, 1, 3), 3)
	state, _ = useSpellOn(t, state, 1)
	require.Equal(t, lasting(spelltest.MultipleDice(t, 2, 1), 1), state.Participants[1].LastingSpellEffect)

	state, events := apply(t, state, matchtest.NewRollingRandom(5, 5), &match.RollCommand{By: matchtest.Index(t, 1)})

	rolled := eventOf[*match.DiceRolledEvent](t, events)
	require.Equal(t, []int{5, 5}, rolled.Dice)
	require.Equal(t, 10, state.Dice)
}

func TestDecide_LastingEffectOfTwoTurnsLosesOneTurnAtEachEnd(t *testing.T) {
	state, _ := useSpellOn(t, rollingWithLastingSpell(t, spelltest.FixedDice(t, 6, 2)), 1)
	state, _ = apply(t, state, matchtest.NewRandom(), &match.RollCommand{By: matchtest.Index(t, 1)})

	state, events := endTurn(t, state, 1)

	require.Zero(t, countOf[*match.SpellEffectEndedEvent](events))
	require.Equal(t, 1, state.Participants[1].LastingSpellEffect.RemainingTurns)
}

func TestDecide_UseSpellRejectsATargetThatDoesNotFit(t *testing.T) {
	outside := matchtest.Index(t, 5)
	zero := matchtest.Index(t, 0)
	lastingSpell := rollingWithLastingSpell(t, spelltest.FixedDice(t, 6, 1))
	draw := rollingWithSpell(t)
	tests := map[string]struct {
		state  match.Match
		target *match.ParticipantIndex
		err    error
	}{
		"lasting_effect_without_target": {state: lastingSpell, target: nil, err: match.ErrSpellTargetRequired},
		"lasting_effect_outside_target": {state: lastingSpell, target: &outside, err: match.ErrSpellTargetNotAllowed},
		"draw_cards_with_target":        {state: draw, target: &zero, err: match.ErrSpellTargetNotAllowed},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := decideErr(t, tt.state, &match.UseSpellCommand{By: matchtest.Index(t, 1), HandIndex: spellIndex(tt.state), Target: tt.target})
			require.ErrorIs(t, err, tt.err)
		})
	}
}

// 参加者 1 の振る段階。参加者 0 の領地 (1,0)（HP 20）と参加者 1 の領地 (2,0) があり、手札の最後に 20 ダメージのスペル 406
func rollingWithDamage(t *testing.T) match.Match {
	t.Helper()
	state := withTerritory(t, withTerritory(t, newMatch(t, territoryBoard(t), defaultRules(t)), 0, 1, 1), 1, 2, 1)
	state, _ = apply(t, state, matchtest.NewRandom(), &match.DrawCommand{By: matchtest.Index(t, 1)})
	require.Equal(t, match.PhaseRolling, state.Phase)
	return withSpellInHand(t, state, 1, 406, 50, spelltest.Damage(t, 20))
}

func useDamage(t *testing.T, state match.Match, x int) (match.Match, []match.Event) {
	t.Helper()
	position := pos(t, x, 0)
	return apply(t, state, matchtest.NewRandom(), &match.UseSpellCommand{By: matchtest.Index(t, 1), HandIndex: spellIndex(state), TargetPosition: &position})
}

// 自分の領地のクリーチャーも選べる（2026-11-11 オーナー）
func TestMatch_SpellChoicesOfDamageOfferEveryTerritory(t *testing.T) {
	state := rollingWithDamage(t)

	choices := state.SpellChoices()

	require.Contains(t, choices, match.SpellChoice{HandIndex: spellIndex(state), Targets: []match.ParticipantIndex{}, TargetPositions: []board.Position{pos(t, 1, 0), pos(t, 2, 0)}})
}

func TestMatch_DamageSpellIsNotOfferedWithoutCreatures(t *testing.T) {
	state := withSpellInHand(t, rollingWithSpell(t), 1, 406, 50, spelltest.Damage(t, 20))

	for _, choice := range state.SpellChoices() {
		require.NotEqual(t, spellIndex(state), choice.HandIndex)
	}
}

func TestDecide_DamageSpellLowersTheHitPointsOfTheCreature(t *testing.T) {
	state := rollingWithDamage(t)
	state.PlacedCreatures = slices.Clone(state.PlacedCreatures)
	state.PlacedCreatures[0].HitPoints = creaturetest.HitPoints(t, 30)

	state, events := useDamage(t, state, 1)

	damaged := eventOf[*match.CreatureDamagedEvent](t, events)
	require.Equal(t, 20, damaged.Damage)
	require.Equal(t, creaturetest.HitPoints(t, 10), damaged.RemainingHitPoints)
	require.Zero(t, countOf[*match.BoardCreatureDestroyedEvent](events))
	require.Equal(t, creaturetest.HitPoints(t, 10), state.PlacedCreatures[0].HitPoints)
	require.Equal(t, match.PhaseRolling, state.Phase)
}

func TestDecide_DamageSpellDestroysTheCreatureAtZero(t *testing.T) {
	state := rollingWithDamage(t)
	state.LandLevels = append(slices.Clone(state.LandLevels), match.LandLevel{Position: pos(t, 1, 0), Level: 3})

	state, events := useDamage(t, state, 1)

	require.Equal(t, 1, countOf[*match.BoardCreatureDestroyedEvent](events))
	_, occupied := state.TerritoryAt(pos(t, 1, 0))
	require.False(t, occupied)
	require.Len(t, state.PlacedCreatures, 1)
	require.Equal(t, 3, state.LandLevel(pos(t, 1, 0)))
	require.Equal(t, []card.Ref{cardtest.CreatureRef(t, 201)}, state.Participants[0].DiscardPile)
}

func TestDecide_UseSpellRejectsATargetPositionThatDoesNotFit(t *testing.T) {
	own := pos(t, 2, 0)
	empty := pos(t, 3, 0)
	damage := rollingWithDamage(t)
	draw := rollingWithSpell(t)
	tests := map[string]struct {
		state    match.Match
		position *board.Position
		err      error
	}{
		"damage_without_position":  {state: damage, position: nil, err: match.ErrSpellTargetPositionRequired},
		"damage_on_vacant_land":    {state: damage, position: &empty, err: match.ErrSpellTargetPositionNotAllowed},
		"draw_cards_with_position": {state: draw, position: &own, err: match.ErrSpellTargetPositionNotAllowed},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := decideErr(t, tt.state, &match.UseSpellCommand{By: matchtest.Index(t, 1), HandIndex: spellIndex(tt.state), TargetPosition: tt.position})
			require.ErrorIs(t, err, tt.err)
		})
	}
}

func TestDecide_DamageSpellCanTargetYourOwnCreature(t *testing.T) {
	state := rollingWithDamage(t)

	state, events := useDamage(t, state, 2)

	require.Equal(t, pos(t, 2, 0), eventOf[*match.CreatureDamagedEvent](t, events).Position)
	require.Equal(t, 1, countOf[*match.BoardCreatureDestroyedEvent](events))
	require.Equal(t, []card.Ref{cardtest.Ref(t, card.SpellEntityType, 406), cardtest.CreatureRef(t, 201)}, state.Participants[1].DiscardPile)
}

// 戦闘中のランダム（AT・HP の上書き。範囲）
func randomStats(t *testing.T, minimum, maximum int) []effect.Effect {
	t.Helper()
	return []effect.Effect{effecttest.AttackOverrideRange(t, minimum, maximum), effecttest.HitPointsOverrideRange(t, minimum, maximum)}
}

func defendWithRandom(t *testing.T, state match.Match, random match.Random) (match.Match, []match.Event) {
	t.Helper()
	return apply(t, state, random, &match.DefendCommand{By: matchtest.Index(t, 0)})
}

func TestDecide_HitPointsRolledByBattleRandomStatsAbsorbDamageAndDoNotRemainAfterTheBattle(t *testing.T) {
	state := withBattlers(t, invade(t, invadingMatch(t)), 10, 20, 5, 20)
	state = withCreatureEffects(t, state, nil, randomStats(t, 10, 70))

	state, _ = defendWithRandom(t, state, matchtest.NewSequenceRandom(0, 25))

	// 引いた HP 35 に 10 のダメージ。戦闘の後は戦闘の前の 20 に戻る
	require.Equal(t, creaturetest.HitPoints(t, 20), state.PlacedCreatures[0].HitPoints)
}

// 「破壊されたとき」の効果は、破壊のイベントの直後に効果の並びの順に発動し、結果が続く（core#182）
func TestDecide_InvadingCreatureReturnsToTheHandRightAfterItIsDestroyed(t *testing.T) {
	state := withBattlers(t, invade(t, invadingMatch(t)), 5, 10, 10, 20)
	state = withCreatureEffects(t, state, []effect.Effect{effecttest.ReturnToHandOnDestruction(t, effect.DestructionInvading, effect.DestructionDefending)}, nil)
	handBefore := len(state.Participants[1].Hand)

	state, events := defend(t, state)

	at := slices.IndexFunc(events, func(event match.Event) bool { _, ok := event.(*match.InvadingCreatureDestroyedEvent); return ok })
	require.GreaterOrEqual(t, at, 0)
	invader := cardtest.CreatureRef(t, 201)
	require.Equal(t, &match.DestructionCreatureEffectActivatedEvent{EventMeta: events[at+1].GetMeta(), Participant: matchtest.Index(t, 1), Owner: matchtest.Index(t, 1), Card: invader, Effect: effect.KindReturnToHandOnDestruction}, events[at+1])
	require.Equal(t, &match.CardReturnedToHandEvent{EventMeta: events[at+2].GetMeta(), Participant: matchtest.Index(t, 1), Owner: matchtest.Index(t, 1), Card: invader}, events[at+2])
	require.IsType(t, &match.InvasionFailedEvent{}, events[at+3])
	require.Len(t, state.Participants[1].Hand, handBefore+1)
	require.Equal(t, invader, state.Participants[1].Hand[handBefore])
	require.Empty(t, state.Participants[1].DiscardPile)
}

// 手札が上限でも戻り、上限は持ち主のターンの終わりに捨てて合わせる（2026-11-15 オーナー）
func TestDecide_DestroyedCreatureReturnsEvenToAFullHand(t *testing.T) {
	state := withBattlers(t, invade(t, invadingMatch(t)), 5, 10, 10, 20)
	state = withCreatureEffects(t, state, []effect.Effect{effecttest.ReturnToHandOnDestruction(t, effect.DestructionInvading)}, nil)
	state.Participants = slices.Clone(state.Participants)
	state.Participants[1].Hand = refs(t, 101, 102, 103, 104, 105, 106)

	state, events := defend(t, state)

	require.Equal(t, 1, countOf[*match.CardReturnedToHandEvent](events))
	// 戻った後、ターンの終わりに上限を超えた分を捨てる段階になる
	require.Len(t, state.Participants[1].Hand, 7)
	require.Equal(t, 1, countOf[*match.HandLimitExceededEvent](events))
	require.Empty(t, state.Participants[1].DiscardPile)
}

func TestDecide_DestructionEffectActivatesOnlyOnTheChosenDestructions(t *testing.T) {
	state := withBattlers(t, invade(t, invadingMatch(t)), 5, 10, 10, 20)
	state = withCreatureEffects(t, state, []effect.Effect{effecttest.ReturnToHandOnDestruction(t, effect.DestructionDefending, effect.DestructionBoard)}, nil)

	state, events := defend(t, state)

	require.Zero(t, countOf[*match.DestructionCreatureEffectActivatedEvent](events))
	require.Equal(t, []card.Ref{cardtest.CreatureRef(t, 201)}, state.Participants[1].DiscardPile)
}

func TestDecide_DefendingCreatureGivesItsOwnerMagicBeforeTheInvasionSucceeds(t *testing.T) {
	state := withBattlers(t, invade(t, invadingMatch(t)), 30, 20, 5, 10)
	state = withCreatureEffects(t, state, nil, []effect.Effect{effecttest.GainMagicOnDestruction(t, 200, effect.DestructionDefending)})
	magicBefore := state.Participants[0].Magic

	state, events := defend(t, state)

	received := slices.IndexFunc(events, func(event match.Event) bool { _, ok := event.(*match.MagicReceivedEvent); return ok })
	succeeded := slices.IndexFunc(events, func(event match.Event) bool { _, ok := event.(*match.InvasionSucceededEvent); return ok })
	require.Positive(t, received)
	require.Less(t, received, succeeded)
	require.Equal(t, &match.MagicReceivedEvent{EventMeta: events[received].GetMeta(), Participant: matchtest.Index(t, 1), Owner: matchtest.Index(t, 0), Magic: 200}, events[received])
	require.Equal(t, magicBefore+200, state.Participants[0].Magic)
}

func TestDecide_CreatureDestroyedOnTheBoardActivatesItsDestructionEffects(t *testing.T) {
	state := rollingWithDamage(t)
	state.PlacedCreatures = slices.Clone(state.PlacedCreatures)
	state.PlacedCreatures[0].Base.Effects = effecttest.Effects(t,
		effecttest.GainMagicOnDestruction(t, 200, effect.DestructionBoard), effecttest.ReturnToHandOnDestruction(t, effect.DestructionBoard))
	magicBefore := state.Participants[0].Magic

	state, events := useDamage(t, state, 1)

	require.Equal(t, 2, countOf[*match.DestructionCreatureEffectActivatedEvent](events))
	require.Equal(t, 1, countOf[*match.MagicReceivedEvent](events))
	require.Equal(t, 1, countOf[*match.CardReturnedToHandEvent](events))
	require.Equal(t, magicBefore+200, state.Participants[0].Magic)
	require.Contains(t, state.Participants[0].Hand, cardtest.CreatureRef(t, 201))
	require.Empty(t, state.Participants[0].DiscardPile)
}
