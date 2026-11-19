package npcdecision_test

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match/matchtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/matchaction"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar/npcavatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdecision"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck/npcdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset/npcmindsettest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
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
	return npcParticipantWith(t, avatar, npcmindset.DefaultActionWeights(), npcmindset.DefaultCriterionWeights())
}

// 思考パターンの写しの重みを指定した NPC の参加者。重みは Choose がここから読む
func npcParticipantWith(t *testing.T, avatar int64, weights npcmindset.ActionWeights, criterionWeights npcmindset.CriterionWeights) match.Participant {
	t.Helper()
	mindset := match.NPCMindset{ID: npcmindsettest.ID(t, avatar+200), ActionWeights: weights, CriterionWeights: criterionWeights}
	return match.NewNPCParticipant(npcavatartest.ID(t, avatar), npcdecktest.ID(t, avatar+100), mindset, []match.DeckCard{
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
func playToEnd(t *testing.T, seed uint64, weights npcmindset.ActionWeights, criterionWeights npcmindset.CriterionWeights) (match.Match, map[string]int) {
	t.Helper()
	random := rand.New(rand.NewPCG(seed, seed))
	cmd := &match.StartCommand{
		ID: matchtest.ID(t, 40), Participants: []match.Participant{npcParticipantWith(t, 1, weights, criterionWeights), npcParticipantWith(t, 2, weights, criterionWeights), npcParticipantWith(t, 3, weights, criterionWeights)},
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
		decided, err = npcdecision.Choose(current, by, random)
		require.NoError(t, err)
		chosen[fmt.Sprintf("%T", decided)]++
	}
	require.FailNow(t, "the match did not end")
	return match.Match{}, nil
}

func TestChoose_PlaysMatchesOfOnlyNPCsToTheEndWithCommandsDecideAccepts(t *testing.T) {
	for _, seed := range []uint64{1, 2, 3, 4, 5} {
		t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
			ended, _ := playToEnd(t, seed, npcmindset.DefaultActionWeights(), npcmindset.DefaultCriterionWeights())

			require.NotNil(t, ended.Result)
			// 戦利品を選ぶプレイヤーがいないので、終わった時点で全員が受け取ったことになる
			require.True(t, ended.RewardsCompleted)
		})
	}
}

// 割合は見ず、どの種類も NPC の選択肢として届くことだけを確かめる（2026-11-17 オーナー。core#179）。
// 採点基準の重みをすべて 0 にするのは、既定の重みでは交換のように 4 つの基準で点の付かない種類が選ばれず（core#190）、
// 採点の振る舞いは仕様のテストで見るため。重みがすべて 0 なら候補はどれも同じ点になり、均等に選ぶ（完全ランダム）
func TestChoose_ChoosesEveryKindOfCommandAcrossMatchesWhenCriteriaAreWeightedZero(t *testing.T) {
	chosen := map[string]int{}
	for _, seed := range []uint64{1, 2, 3, 4, 5} {
		_, counts := playToEnd(t, seed, npcmindset.DefaultActionWeights(), npcmindset.CriterionWeights{})
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

// 重みが 0 の種類は、ほかに選べる種類があれば選ばない。点の付かない段階（振る段階はスペルを使う・振る）で効く
// （配置・侵略の段階は採点するので、種類の重みは効かない。TestChoose_ScoredPhasesIgnoreWeightsOfActionKinds）
func TestChoose_NeverChoosesActionsWeightedZeroWhenOthersCanBeChosen(t *testing.T) {
	rolling := rollingWithSpell(t)
	by, _ := rolling.AwaitingParticipant()
	weights := npcmindset.DefaultActionWeights()
	weights[matchaction.ActionUseSpell] = 0
	rolling = withMindset(rolling, by, weights, npcmindset.DefaultCriterionWeights())

	for seed := range uint64(20) {
		cmd, err := npcdecision.Choose(rolling, by, rand.New(rand.NewPCG(seed, seed)))

		require.NoError(t, err)
		require.Equal(t, "*match.RollCommand", kindOf(cmd))
	}
}

// 選べる種類の重みがすべて 0 でも、均等に選んで止まらない
func TestChoose_ChoosesEvenlyWhenEveryWeightIsZero(t *testing.T) {
	ended, _ := playToEnd(t, 1, npcmindset.ActionWeights{}, npcmindset.DefaultCriterionWeights())

	require.NotNil(t, ended.Result)
}

// 重みの合計から引いた値を、並びの順に重みを積んで当てる。振る段階の並びはスペルを使う → 振る
func TestChoose_ChoosesTheActionWhoseWeightRangeContainsTheDrawnValue(t *testing.T) {
	rolling := rollingWithSpell(t)
	require.Equal(t, []matchaction.Action{matchaction.ActionUseSpell, matchaction.ActionRollDice}, rolling.ActionChoices())
	by, _ := rolling.AwaitingParticipant()
	rolling = withMindset(rolling, by, npcmindset.ActionWeights{matchaction.ActionUseSpell: 1, matchaction.ActionRollDice: 3}, npcmindset.DefaultCriterionWeights())
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
			cmd, err := npcdecision.Choose(rolling, by, matchtest.NewSequenceRandom(tt.drawn, 0, 0))

			require.NoError(t, err)
			require.Equal(t, tt.want, fmt.Sprintf("%T", cmd))
		})
	}
}

// 全員 NPC の対戦を始め、振る段階で使えるスペルが手札にある状態まで進める
func rollingWithSpell(t *testing.T) match.Match {
	t.Helper()
	// スペルを使わずに進めるため、引く・振る・終わる・払うだけの重み
	progressWeights := npcmindset.ActionWeights{matchaction.ActionDrawCard: 1, matchaction.ActionRollDice: 1, matchaction.ActionEndTurn: 1, matchaction.ActionPayToll: 1}
	random := rand.New(rand.NewPCG(1, 1))
	cmd := &match.StartCommand{
		ID: matchtest.ID(t, 40), Participants: []match.Participant{npcParticipantWith(t, 1, progressWeights, npcmindset.DefaultCriterionWeights()), npcParticipantWith(t, 2, progressWeights, npcmindset.DefaultCriterionWeights())},
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
		decided, err = npcdecision.Choose(current, by, random)
		require.NoError(t, err)
	}
	require.FailNow(t, "no rolling phase with a spell")
	return match.Match{}
}

// 全員 NPC の 2 人の対戦を、条件に合う状態まで進める（種を変えながら探す）
func advanceUntil(t *testing.T, found func(current match.Match, by match.ParticipantIndex) bool) match.Match {
	t.Helper()
	return advanceUntilOn(t, ringBoard(t), found)
}

func advanceUntilOn(t *testing.T, matchBoard match.Board, found func(current match.Match, by match.ParticipantIndex) bool) match.Match {
	t.Helper()
	for seed := uint64(1); seed <= 300; seed++ {
		random := rand.New(rand.NewPCG(seed, seed))
		cmd := &match.StartCommand{
			ID: matchtest.ID(t, 40), Participants: []match.Participant{npcParticipant(t, 1), npcParticipant(t, 2)},
			Board: matchBoard, Rules: matchtest.RulesOf(t, 3000, 600, 300, 100, 10, 1, 6, 5),
		}
		var state match.State = match.InitialState()
		var decided match.Command = cmd
		for range 400 {
			events, err := match.Decide(state, decided, startedAt, random)
			require.NoError(t, err)
			for _, event := range events {
				state, err = match.Evolve(state, event)
				require.NoError(t, err)
			}
			current := state.(match.Match)
			if current.Phase == match.PhaseEnded {
				break
			}
			by, _ := current.AwaitingParticipant()
			if found(current, by) {
				return current
			}
			decided, err = npcdecision.Choose(current, by, random)
			require.NoError(t, err)
		}
	}
	require.FailNow(t, "no state found")
	return match.Match{}
}

func kindOf(cmd match.Command) string {
	return fmt.Sprintf("%T", cmd)
}

// 侵略する側が止まっている他人の領地の、クリーチャーの性能を差し替える（侵略の結果を確実にするため）
func withDefender(t *testing.T, current match.Match, attack, hitPoints int) match.Match {
	t.Helper()
	territory, ok := current.TerritoryAt(current.Participants[current.TurnOrder[current.Turn]].Position)
	require.True(t, ok)
	current.PlacedCreatures = append([]match.PlacedCreature(nil), current.PlacedCreatures...)
	for i := range current.PlacedCreatures {
		if current.PlacedCreatures[i].ID == territory.Creature {
			current.PlacedCreatures[i].Attack = creaturetest.Attack(t, attack)
			current.PlacedCreatures[i].HitPoints = creaturetest.HitPoints(t, hitPoints)
		}
	}
	return current
}

// 位置のクリーチャーの性能を差し替える
func withCreatureAt(t *testing.T, current match.Match, position board.Position, attack, hitPoints int) match.Match {
	t.Helper()
	territory, ok := current.TerritoryAt(position)
	require.True(t, ok)
	current.PlacedCreatures = append([]match.PlacedCreature(nil), current.PlacedCreatures...)
	for i := range current.PlacedCreatures {
		if current.PlacedCreatures[i].ID == territory.Creature {
			current.PlacedCreatures[i].Attack = creaturetest.Attack(t, attack)
			current.PlacedCreatures[i].HitPoints = creaturetest.HitPoints(t, hitPoints)
		}
	}
	return current
}

func ownedByOther(current match.Match, by match.ParticipantIndex, position board.Position) bool {
	territory, ok := current.TerritoryAt(position)
	if !ok {
		return false
	}
	owner, ok := current.TerritoryOwner(territory)
	return ok && owner != by
}

func invadingWithPlainCreatures(current match.Match, _ match.ParticipantIndex) bool {
	return current.Phase == match.PhaseInvading && len(current.InvadeChoices()) > 0
}

func TestChoose_PlacesACreatureOnAVacantLandItCanAfford(t *testing.T) {
	current := advanceUntil(t, func(current match.Match, _ match.ParticipantIndex) bool {
		return current.Phase == match.PhasePlacing && len(current.PlaceChoices()) > 0 && len(current.ActionChoices()) == 2
	})
	by, _ := current.AwaitingParticipant()

	cmd, err := npcdecision.Choose(current, by, matchtest.NewSequenceRandom(0, 0, 0))

	require.NoError(t, err)
	require.Equal(t, "*match.PlaceCreatureCommand", kindOf(cmd))
}

func TestChoose_PaysTheTollInsteadOfInvadingWithACreatureThatCertainlyLoses(t *testing.T) {
	current := advanceUntil(t, invadingWithPlainCreatures)
	current = withDefender(t, current, 500, 500)
	by, _ := current.AwaitingParticipant()

	for seed := range uint64(8) {
		cmd, err := npcdecision.Choose(current, by, rand.New(rand.NewPCG(seed, seed)))

		require.NoError(t, err)
		require.Equal(t, "*match.PayTollCommand", kindOf(cmd))
	}
}

func TestChoose_InvadesWhenTheInvasionCertainlyWins(t *testing.T) {
	current := advanceUntil(t, invadingWithPlainCreatures)
	current = withDefender(t, current, 0, 1)
	by, _ := current.AwaitingParticipant()

	for seed := range uint64(8) {
		cmd, err := npcdecision.Choose(current, by, rand.New(rand.NewPCG(seed, seed)))

		require.NoError(t, err)
		require.Equal(t, "*match.InvadeCommand", kindOf(cmd))
	}
}

// 負けて元の領地に戻るだけの移動の侵略は、費用が無くても、戦った傷（クリーチャーの消耗）の分だけ何もしないより悪い
// （消耗を数えないと何もしないと同じ点になり、くじで選ばれていた。core#191）
func TestChoose_DoesNotMoveToInvadeWhenTheCreatureOnlyComesBackHurt(t *testing.T) {
	current := advanceUntil(t, func(current match.Match, by match.ParticipantIndex) bool {
		if current.Phase != match.PhaseTerritoryCommanding {
			return false
		}
		for _, choice := range current.MoveChoices() {
			if ownedByOther(current, by, choice.To) {
				return true
			}
		}
		return false
	})
	by, _ := current.AwaitingParticipant()
	for _, choice := range current.MoveChoices() {
		if ownedByOther(current, by, choice.To) {
			// 倒せず、侵略したクリーチャーも倒れず、少しだけ傷を負う守り手
			current = withCreatureAt(t, current, choice.To, 1, 500)
		}
	}
	// 費用のかかる領地コマンド（レベルアップ・交換・属性の変更）を選べないようにし、費用の無い移動と終わりだけを比べる
	current.Participants = append([]match.ParticipantState(nil), current.Participants...)
	current.Participants[by].Magic = 0

	for seed := range uint64(8) {
		cmd, err := npcdecision.Choose(current, by, rand.New(rand.NewPCG(seed, seed)))

		require.NoError(t, err)
		if move, ok := cmd.(*match.MoveCreatureCommand); ok {
			require.False(t, ownedByOther(current, by, move.To))
		}
	}
}

// 相手の手札で一番強い攻めに耐えきれない領地のクリーチャーは、耐えきれる手札のクリーチャーに交換する（交換の費用より、
// 取られる見込みの領地の価値のほうが大きい。守備評価が無いと、交換は費用の分だけ何もしないより悪かった。core#194）
func TestChoose_ExchangesTheCreatureThatCannotHoldItsTerritory(t *testing.T) {
	sturdy := cardtest.CreatureRef(t, 201) // HP 40
	current := advanceUntil(t, func(current match.Match, _ match.ParticipantIndex) bool {
		return current.Phase == match.PhaseTerritoryCommanding && slices.ContainsFunc(current.ExchangeChoices(), func(choice match.ExchangeChoice) bool {
			by, _ := current.AwaitingParticipant()
			return current.Participants[by].Hand[choice.HandIndex] == sturdy
		})
	})
	by, _ := current.AwaitingParticipant()
	var position board.Position
	for _, choice := range current.ExchangeChoices() {
		if current.Participants[by].Hand[choice.HandIndex] == sturdy {
			position = choice.Position
		}
	}
	// AT 0 で、移動の侵略で勝って逃げる道も外す
	current = withCreatureAt(t, current, position, 0, 1)
	current.Participants = append([]match.ParticipantState(nil), current.Participants...)
	// 交換だけを払えるようにし（レベルアップ・属性の変更で地形効果を足す道を外す）、相手の攻めを AT 20 のクリーチャー 1 枚にする
	current.Participants[by].Magic = 20
	for i := range current.Participants {
		if i != by.Int() {
			current.Participants[i].Hand = []card.Ref{cardtest.CreatureRef(t, 203)}
		}
	}

	for seed := range uint64(8) {
		cmd, err := npcdecision.Choose(current, by, rand.New(rand.NewPCG(seed, seed)))

		require.NoError(t, err)
		exchange, ok := cmd.(*match.ExchangeCreatureCommand)
		require.True(t, ok)
		require.Equal(t, position, exchange.Position)
	}
}

// 3×3 の外周の輪に、真ん中の縦の道 (1,0)-(1,1)-(1,2) を足したボード。(1,0) と (1,2) が分かれ道になる
func forkBoard(t *testing.T) match.Board {
	t.Helper()
	ringed := ringBoard(t)
	ringed.Tiles = append(append([]board.Tile(nil), ringed.Tiles...), boardtest.Tile(t, 1, 1, "land", "fire"))
	ringed.Connections = append(append([]board.Connection(nil), ringed.Connections...), boardtest.Connection(t, 1, 0, 1, 1), boardtest.Connection(t, 1, 1, 1, 2))
	return ringed
}

// 行き先は、止まった後の一番よい行動まで進めた点で選ぶ。残り 1 歩で、強い守り手の他人の領地（通行料を払うしかない）と空き地に分かれるなら、空き地へ行く
func TestChoose_ChoosesThePathWhoseStopIsBetter(t *testing.T) {
	stops := func(current match.Match, by match.ParticipantIndex) (enemy, vacant board.Position, ok bool) {
		var foundEnemy, foundVacant bool
		for _, to := range current.PathChoices() {
			land := slices.ContainsFunc(current.Board.Tiles, func(tile board.Tile) bool { return tile.Position == to && tile.Kind == board.TileKindLand })
			switch {
			case ownedByOther(current, by, to):
				enemy, foundEnemy = to, true
			case land:
				if _, occupied := current.TerritoryAt(to); !occupied {
					vacant, foundVacant = to, true
				}
			}
		}
		return enemy, vacant, foundEnemy && foundVacant
	}
	current := advanceUntilOn(t, forkBoard(t), func(current match.Match, by match.ParticipantIndex) bool {
		_, _, ok := stops(current, by)
		return current.Phase == match.PhaseChoosingPath && current.RemainingSteps == 1 && ok
	})
	by, _ := current.AwaitingParticipant()
	enemy, vacant, _ := stops(current, by)
	current = withCreatureAt(t, current, enemy, 500, 500)

	for seed := range uint64(8) {
		cmd, err := npcdecision.Choose(current, by, rand.New(rand.NewPCG(seed, seed)))

		require.NoError(t, err)
		require.Equal(t, &match.ChoosePathCommand{By: by, To: vacant}, cmd)
	}
}

// 同じカードが 2 枚ある手札の、どちらを置いても点は同じ
func placingWithTwinCards(t *testing.T) (match.Match, match.ParticipantIndex) {
	t.Helper()
	current := advanceUntil(t, func(current match.Match, _ match.ParticipantIndex) bool {
		return current.Phase == match.PhasePlacing && len(current.PlaceChoices()) > 0 && len(current.ActionChoices()) == 2
	})
	by, _ := current.AwaitingParticipant()
	twin := cardtest.CreatureRef(t, 201)
	current.Participants = append([]match.ParticipantState(nil), current.Participants...)
	current.Participants[by].Hand = []card.Ref{twin, twin}
	require.Equal(t, []int{0, 1}, current.PlaceChoices())
	return current, by
}

func TestChoose_ChoosesEvenlyAmongCandidatesWithTheSameScore(t *testing.T) {
	current, by := placingWithTwinCards(t)

	chosen := map[int]bool{}
	for _, drawn := range []int{0, 1} {
		cmd, err := npcdecision.Choose(current, by, matchtest.NewSequenceRandom(drawn))

		require.NoError(t, err)
		place, ok := cmd.(*match.PlaceCreatureCommand)
		require.True(t, ok)
		chosen[place.HandIndex] = true
	}
	require.Equal(t, map[int]bool{0: true, 1: true}, chosen)
}

func TestChoose_IgnoresCriteriaWeightedZero(t *testing.T) {
	current, by := placingWithTwinCards(t)
	tests := []struct {
		name    string
		weights npcmindset.CriterionWeights
		draws   []int
		kinds   map[string]bool
	}{
		// 3 つの候補（配置 2 つと終わり）が同じ点になり、均等に選ばれる
		{name: "all_zero", weights: weightsOnly(), draws: []int{0, 1, 2}, kinds: map[string]bool{"*match.PlaceCreatureCommand": true, "*match.EndTurnCommand": true}},
		// 領地の数だけを見れば、置くほうが必ず選ばれる
		{name: "only_territory_count", weights: weightsOnly(npcmindset.CriterionTerritoryCount), draws: []int{0, 1}, kinds: map[string]bool{"*match.PlaceCreatureCommand": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chosen := map[string]bool{}
			for _, drawn := range tt.draws {
				cmd, err := npcdecision.Choose(withMindset(current, by, npcmindset.DefaultActionWeights(), tt.weights), by, matchtest.NewSequenceRandom(drawn))

				require.NoError(t, err)
				chosen[kindOf(cmd)] = true
			}
			require.Equal(t, tt.kinds, chosen)
		})
	}
}

// 重みの種類は、点の付かない段階（ここでは振る段階）だけで効く。採点する段階では、種類の重みが 0 でも点の高い候補を選ぶ
func TestChoose_ScoredPhasesIgnoreWeightsOfActionKinds(t *testing.T) {
	current := advanceUntil(t, func(current match.Match, _ match.ParticipantIndex) bool {
		return current.Phase == match.PhasePlacing && len(current.PlaceChoices()) > 0 && len(current.ActionChoices()) == 2
	})
	by, _ := current.AwaitingParticipant()

	current = withMindset(current, by, npcmindset.ActionWeights{}, npcmindset.DefaultCriterionWeights())

	cmd, err := npcdecision.Choose(current, by, matchtest.NewSequenceRandom(0))

	require.NoError(t, err)
	require.Equal(t, "*match.PlaceCreatureCommand", kindOf(cmd))
}

// 候補を試すために当てた状態は、元の状態を変えない（match.Evolve が参照型を壊さない前提を、ここで固める）
func TestChoose_DoesNotChangeTheCurrentState(t *testing.T) {
	phases := []match.Phase{match.PhasePlacing, match.PhaseInvading, match.PhaseAttacking, match.PhaseDefending, match.PhaseTerritoryCommanding, match.PhaseEnding}
	for _, phase := range phases {
		t.Run(string(phase), func(t *testing.T) {
			current := advanceUntil(t, func(current match.Match, _ match.ParticipantIndex) bool { return current.Phase == phase })
			by, _ := current.AwaitingParticipant()
			before, err := json.Marshal(current)
			require.NoError(t, err)

			_, err = npcdecision.Choose(current, by, rand.New(rand.NewPCG(1, 1)))

			require.NoError(t, err)
			after, err := json.Marshal(current)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after))
		})
	}
}

// 指定した基準だけ重みが 1 で、ほかは 0 の重み
func weightsOnly(criteria ...npcmindset.Criterion) npcmindset.CriterionWeights {
	weights := npcmindset.CriterionWeights{}
	for _, criterion := range criteria {
		weights[criterion] = 1
	}
	return weights
}

// 重みは参加者に写した思考パターンから読むので、重みを変えるテストは、写しを変えた参加者で対戦を作る
// （対戦の途中から変えるテストは、この関数で NPC の参加者の写しだけを差し替える）
func withMindset(current match.Match, by match.ParticipantIndex, weights npcmindset.ActionWeights, criterionWeights npcmindset.CriterionWeights) match.Match {
	current.Participants = append([]match.ParticipantState(nil), current.Participants...)
	npc := *current.Participants[by].Participant.NPC
	npc.Mindset.ActionWeights = weights
	npc.Mindset.CriterionWeights = criterionWeights
	current.Participants[by].Participant.NPC = &npc
	return current
}

func TestChoose_RejectsParticipantsThatAreNotNPCs(t *testing.T) {
	current := advanceUntil(t, func(current match.Match, _ match.ParticipantIndex) bool { return current.Phase == match.PhaseRolling })
	by, _ := current.AwaitingParticipant()
	current.Participants = append([]match.ParticipantState(nil), current.Participants...)
	current.Participants[by].Participant = match.NewPlayerParticipant(playertest.ID(t, 1), avatartest.ID(t, 2), decktest.ID(t, 3), nil)

	_, notNPC := npcdecision.Choose(current, by, rand.New(rand.NewPCG(1, 1)))
	_, outOfRange := npcdecision.Choose(current, matchtest.Index(t, 9), rand.New(rand.NewPCG(1, 1)))

	require.ErrorIs(t, notNPC, npcdecision.ErrNotNPC)
	require.ErrorIs(t, outOfRange, npcdecision.ErrNotNPC)
}
