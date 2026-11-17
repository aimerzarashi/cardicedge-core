package npcmind_test

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature/creaturetest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect/effecttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match/matchtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar/npcavatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck/npcdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmind"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell/spelltest"
)

var startedAt = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

// 拠点 (0,0) を含む 3×3 の外周 8 マスの輪。チェックポイントは (2,2)、ほかは属性の違う土地
func ringBoard(t *testing.T) match.Board {
	t.Helper()
	ring := []struct {
		x, y    int
		kind    string
		element string
	}{
		{0, 0, "base", ""}, {1, 0, "land", "fire"}, {2, 0, "land", "fire"}, {2, 1, "land", "water"},
		{2, 2, "checkpoint", ""}, {1, 2, "land", "earth"}, {0, 2, "land", "wind"}, {0, 1, "land", "neutral"},
	}
	var tiles []board.Tile
	var connections []board.Connection
	for i, tile := range ring {
		tiles = append(tiles, boardtest.Tile(t, tile.x, tile.y, tile.kind, tile.element))
		next := ring[(i+1)%len(ring)]
		connections = append(connections, boardtest.Connection(t, tile.x, tile.y, next.x, next.y))
	}
	return match.Board{ID: boardtest.ID(t, 5), Size: boardtest.Size(t, 3, 3), Tiles: tiles, Connections: connections}
}

func creatureCard(t *testing.T, id int64, cost, attack, hitPoints int, effects ...effect.Effect) match.DeckCard {
	t.Helper()
	spec := &match.CreatureSpec{
		Elements: creature.DefaultElements(), Attack: creaturetest.Attack(t, attack), HitPoints: creaturetest.HitPoints(t, hitPoints),
		ItemRestrictions: creature.DefaultItemRestrictions(), PlacementRestrictions: creature.DefaultPlacementRestrictions(),
		Effects: effecttest.Effects(t, effects...),
	}
	cardSpec := match.CardSpec{Cost: cardtest.Cost(t, cost), Rarity: card.RarityNormal, Creature: spec}
	return match.DeckCard{Card: cardtest.CreatureRef(t, id), Quantity: cardtest.DeckQuantity(t, 4), Spec: cardSpec}
}

func itemCard(t *testing.T, id int64) match.DeckCard {
	t.Helper()
	spec := &match.ItemSpec{Category: card.ItemCategoryWeapon, Effects: effecttest.Effects(t, effecttest.AttackModifier(20))}
	cardSpec := match.CardSpec{Cost: cardtest.Cost(t, 20), Rarity: card.RarityNormal, Item: spec}
	return match.DeckCard{Card: cardtest.Ref(t, card.ItemEntityType, id), Quantity: cardtest.DeckQuantity(t, 4), Spec: cardSpec}
}

func spellCard(t *testing.T, id int64, spellEffect effect.SpellEffect) match.DeckCard {
	t.Helper()
	cardSpec := match.CardSpec{Cost: cardtest.Cost(t, 30), Rarity: card.RarityNormal, Spell: &match.SpellSpec{Effect: spellEffect}}
	return match.DeckCard{Card: cardtest.Ref(t, card.SpellEntityType, id), Quantity: cardtest.DeckQuantity(t, 3), Spec: cardSpec}
}

// クリーチャー（援護を持つものを含む）・アイテム・スペル（対象を選ばない・プレイヤーを選ぶ・クリーチャーを選ぶ）の入ったデッキ
func npcParticipant(t *testing.T, avatar int64) match.Participant {
	t.Helper()
	return match.NewNPCParticipant(npcavatartest.ID(t, avatar), npcdecktest.ID(t, avatar+100), []match.DeckCard{
		creatureCard(t, 201, 20, 30, 40),
		creatureCard(t, 202, 40, 50, 30),
		creatureCard(t, 203, 30, 20, 50, effecttest.UseCreatureAsItem(t)),
		itemCard(t, 301),
		spellCard(t, 401, spelltest.DrawCards(t, 1)),
		spellCard(t, 402, spelltest.FixedDice(t, 2, 1)),
		spellCard(t, 403, spelltest.Damage(t, 20)),
	})
}

// 全員 NPC の対戦を、完全ランダム型の選んだコマンドだけで終わりまで進める。選んだコマンドの型を数えて返す
func playToEnd(t *testing.T, seed uint64, weights npcmind.Weights) (match.Match, map[string]int) {
	t.Helper()
	random := rand.New(rand.NewPCG(seed, seed))
	cmd := &match.StartCommand{
		ID: matchtest.ID(t, 40), Participants: []match.Participant{npcParticipant(t, 1), npcParticipant(t, 2), npcParticipant(t, 3)},
		Board: ringBoard(t), Rules: matchtest.RulesOf(t, 3000, 600, 300, 100, 10, 1, 6, 5),
	}
	var state match.State = match.InitialState()
	var decided match.Command = cmd
	chosen := map[string]int{}
	for range 5000 {
		events, err := match.Decide(state, decided, startedAt, random)
		require.NoError(t, err)
		for _, event := range events {
			state, err = match.Evolve(state, event)
			require.NoError(t, err)
		}
		current := state.(match.Match)
		if current.Phase == match.PhaseEnded {
			return current, chosen
		}
		by, ok := current.AwaitingParticipant()
		require.True(t, ok)
		decided, err = npcmind.Choose(current, by, weights, random)
		require.NoError(t, err)
		chosen[fmt.Sprintf("%T", decided)]++
	}
	require.FailNow(t, "the match did not end")
	return match.Match{}, nil
}

func TestChoose_PlaysMatchesOfOnlyNPCsToTheEndWithCommandsDecideAccepts(t *testing.T) {
	for _, seed := range []uint64{1, 2, 3, 4, 5} {
		t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
			ended, _ := playToEnd(t, seed, npcmind.DefaultWeights())

			require.NotNil(t, ended.Result)
			// 戦利品を選ぶプレイヤーがいないので、終わった時点で全員が受け取ったことになる
			require.True(t, ended.RewardsCompleted)
		})
	}
}

// 割合は見ず、どの種類も選ばれうることだけを確かめる（2026-11-17 オーナー。core#179）
func TestChoose_ChoosesEveryKindOfCommandAcrossMatches(t *testing.T) {
	chosen := map[string]int{}
	for _, seed := range []uint64{1, 2, 3, 4, 5} {
		_, counts := playToEnd(t, seed, npcmind.DefaultWeights())
		for kind, count := range counts {
			chosen[kind] += count
		}
	}

	tests := []struct {
		name string
		kind string
	}{
		{name: "draw", kind: "*match.DrawCommand"}, {name: "roll", kind: "*match.RollCommand"},
		{name: "use_spell", kind: "*match.UseSpellCommand"}, {name: "place", kind: "*match.PlaceCreatureCommand"},
		{name: "invade", kind: "*match.InvadeCommand"}, {name: "pay_toll", kind: "*match.PayTollCommand"},
		{name: "attack", kind: "*match.AttackCommand"}, {name: "defend", kind: "*match.DefendCommand"},
		{name: "use_creature_as_item", kind: "*match.UseCreatureAsItemCommand"}, {name: "level_up", kind: "*match.LevelUpLandCommand"},
		{name: "exchange", kind: "*match.ExchangeCreatureCommand"}, {name: "change_element", kind: "*match.ChangeLandElementCommand"},
		{name: "move", kind: "*match.MoveCreatureCommand"}, {name: "end_turn", kind: "*match.EndTurnCommand"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Positive(t, chosen[tt.kind])
		})
	}
}

// 重みが 0 の種類は、ほかに選べる種類があれば選ばない（配置の段階はターンを終えるも、侵略の段階は通行料を払うもいつも選べる）
func TestChoose_NeverChoosesActionsWeightedZeroWhenOthersCanBeChosen(t *testing.T) {
	tests := []struct {
		name        string
		zero        match.Action
		neverChosen string
		alternative string
	}{
		{name: "place_creature", zero: match.ActionPlaceCreature, neverChosen: "*match.PlaceCreatureCommand", alternative: "*match.EndTurnCommand"},
		{name: "invade_territory", zero: match.ActionInvadeTerritory, neverChosen: "*match.InvadeCommand", alternative: "*match.PayTollCommand"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weights := npcmind.DefaultWeights()
			weights[tt.zero] = 0
			chosen := map[string]int{}
			for _, seed := range []uint64{1, 2, 3, 4, 5} {
				_, counts := playToEnd(t, seed, weights)
				for kind, count := range counts {
					chosen[kind] += count
				}
			}

			require.Zero(t, chosen[tt.neverChosen])
			require.Positive(t, chosen[tt.alternative])
		})
	}
}

// 選べる種類の重みがすべて 0 でも、均等に選んで止まらない
func TestChoose_ChoosesEvenlyWhenEveryWeightIsZero(t *testing.T) {
	ended, _ := playToEnd(t, 1, npcmind.Weights{})

	require.NotNil(t, ended.Result)
}

// 重みの合計から引いた値を、並びの順に重みを積んで当てる。振る段階の並びはスペルを使う → 振る
func TestChoose_ChoosesTheActionWhoseWeightRangeContainsTheDrawnValue(t *testing.T) {
	weights := npcmind.Weights{match.ActionUseSpell: 1, match.ActionRollDice: 3}
	rolling := rollingWithSpell(t)
	require.Equal(t, []match.Action{match.ActionUseSpell, match.ActionRollDice}, rolling.ActionChoices())
	by, _ := rolling.AwaitingParticipant()
	tests := []struct {
		name  string
		drawn int
		want  string
	}{
		{name: "first_range", drawn: 0, want: "*match.UseSpellCommand"},
		{name: "second_range_start", drawn: 1, want: "*match.RollCommand"},
		{name: "second_range_end", drawn: 3, want: "*match.RollCommand"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1 つ目は種類、続く値はスペルの対象を選ぶのに使う
			cmd, err := npcmind.Choose(rolling, by, weights, matchtest.NewSequenceRandom(tt.drawn, 0, 0))

			require.NoError(t, err)
			require.Equal(t, tt.want, fmt.Sprintf("%T", cmd))
		})
	}
}

func TestNewWeight_RejectsNegativeValues(t *testing.T) {
	_, negative := npcmind.NewWeight(-1)
	zero, err := npcmind.NewWeight(0)

	require.ErrorIs(t, negative, npcmind.ErrNegativeWeight)
	require.NoError(t, err)
	require.Equal(t, 0, zero.Int())
}

// 全員 NPC の対戦を始め、振る段階で使えるスペルが手札にある状態まで進める
func rollingWithSpell(t *testing.T) match.Match {
	t.Helper()
	random := rand.New(rand.NewPCG(1, 1))
	cmd := &match.StartCommand{
		ID: matchtest.ID(t, 40), Participants: []match.Participant{npcParticipant(t, 1), npcParticipant(t, 2)},
		Board: ringBoard(t), Rules: matchtest.RulesOf(t, 3000, 600, 300, 100, 10, 1, 6, 5),
	}
	var state match.State = match.InitialState()
	var decided match.Command = cmd
	for range 200 {
		events, err := match.Decide(state, decided, startedAt, random)
		require.NoError(t, err)
		for _, event := range events {
			state, err = match.Evolve(state, event)
			require.NoError(t, err)
		}
		current := state.(match.Match)
		if current.Phase == match.PhaseRolling && len(current.SpellChoices()) > 0 {
			return current
		}
		by, _ := current.AwaitingParticipant()
		decided, err = npcmind.Choose(current, by, npcmind.Weights{match.ActionDrawCard: 1, match.ActionRollDice: 1, match.ActionEndTurn: 1, match.ActionPayToll: 1}, random)
		require.NoError(t, err)
	}
	require.FailNow(t, "no rolling phase with a spell")
	return match.Match{}
}
