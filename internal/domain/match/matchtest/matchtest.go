package matchtest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature/creaturetest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules/gamerulestest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
)

// テストでも値の型は生成関数で作る（factory_enforcer）

func ID(t testing.TB, value int64) match.ID {
	t.Helper()
	id, err := match.NewID(value)
	require.NoError(t, err)
	return id
}

// 目の範囲と配る枚数だけを変えたルール（ほかはボードの既定の値）
func Rules(t testing.TB, diceMin, diceMax, startingHandCards int) match.Rules {
	t.Helper()
	return match.NewRules(
		boardtest.DefaultRules(t, 8000, 600, 300, 100, 30, diceMin, diceMax, 100),
		gamerulestest.MaxHandCards(t, 6),
		gamerulestest.StartingHandCards(t, startingHandCards),
		gamerulestest.RewardCardsByRank(t, 3, 2, 1, 1),
		match.DefaultRewardPacks(),
		gamerules.Defaults().Match.LandLevelMultipliers,
		gamerules.Defaults().Match.ChainMultipliers,
		gamerules.Defaults().Match.TollPercentage,
		gamerules.Defaults().Match.RelinquishPercentage,
		gamerules.Defaults().Match.TerrainBonusPerLevel,
		gamerules.Defaults().Match.AdjacentTerritoryBonus,
		gamerules.Defaults().Match.LandElementChangeMultiplier,
		gamerules.Defaults().Match.MultiLandElementChangeMultiplier,
	)
}

// 混ぜる並びを逆順にする乱数の元（順番も山札も、席の並び・デッキの並びの逆になる）
type Random struct{}

func NewRandom() Random {
	return Random{}
}

func (Random) Shuffle(n int, swap func(i, j int)) {
	for i := range n / 2 {
		swap(i, n-1-i)
	}
}

func Index(t testing.TB, value int) match.ParticipantIndex {
	t.Helper()
	index, err := match.NewParticipantIndex(value)
	require.NoError(t, err)
	return index
}

func (Random) IntN(int) int {
	return 0
}

// 決めた目を順に返す乱数の元（目の最小が 1 のルール用）。Shuffle は Random と同じ逆順
type RollingRandom struct {
	Random
	faces []int
}

func NewRollingRandom(faces ...int) *RollingRandom {
	return &RollingRandom{faces: faces}
}

func (r *RollingRandom) IntN(int) int {
	if len(r.faces) == 0 {
		panic("matchtest: no dice faces left")
	}
	face := r.faces[0]
	r.faces = r.faces[1:]
	return face - 1
}

// IntN の戻り値をそのまま順に返す乱数の元（範囲の中の位置を決めたいとき用。RollingRandom は目から 1 を引いて返す）
type SequenceRandom struct {
	Random
	values []int
}

func NewSequenceRandom(values ...int) *SequenceRandom {
	return &SequenceRandom{values: values}
}

func (r *SequenceRandom) IntN(int) int {
	if len(r.values) == 0 {
		panic("matchtest: no random values left")
	}
	value := r.values[0]
	r.values = r.values[1:]
	return value
}

// 最大の目を返す乱数の元
type HighestRandom struct {
	Random
}

func NewHighestRandom() HighestRandom {
	return HighestRandom{}
}

func (HighestRandom) IntN(n int) int {
	return n - 1
}

// すべての値を指定するルール（ボードの既定の値は board.DefaultRules で受け取る）
func RulesOf(t testing.TB, targetMagic, startingMagic, lapBonus, checkpointBonus, roundLimit, diceMin, diceMax, startingHandCards int) match.Rules {
	t.Helper()
	return match.NewRules(
		boardtest.DefaultRules(t, targetMagic, startingMagic, lapBonus, checkpointBonus, roundLimit, diceMin, diceMax, 100),
		gamerulestest.MaxHandCards(t, 6),
		gamerulestest.StartingHandCards(t, startingHandCards),
		gamerulestest.RewardCardsByRank(t, 3, 2, 1, 1),
		match.DefaultRewardPacks(),
		gamerules.Defaults().Match.LandLevelMultipliers,
		gamerules.Defaults().Match.ChainMultipliers,
		gamerules.Defaults().Match.TollPercentage,
		gamerules.Defaults().Match.RelinquishPercentage,
		gamerules.Defaults().Match.TerrainBonusPerLevel,
		gamerules.Defaults().Match.AdjacentTerritoryBonus,
		gamerules.Defaults().Match.LandElementChangeMultiplier,
		gamerules.Defaults().Match.MultiLandElementChangeMultiplier,
	)
}

func PlacedCreatureID(t testing.TB, value int) match.PlacedCreatureID {
	t.Helper()
	id, err := match.NewPlacedCreatureID(value)
	require.NoError(t, err)
	return id
}

// 戦闘の入力を組み立てる道具。素の入力（攻める側はカード 101・守る側は 201 で、どちらも無属性・AT 10・HP 40、
// アイテム・能力・隣の領地なし、土地は火のレベル 1、支援効果と地形効果はゲーム全体の既定の値）から、違うところだけを変える
type BattleInputBuilder struct {
	t     testing.TB
	input match.BattleInput
}

func Battle(t testing.TB) *BattleInputBuilder {
	t.Helper()
	combatant := func(id int64) match.BattleCombatant {
		spec := match.CreatureSpec{
			Elements: creature.DefaultElements(), Attack: creaturetest.Attack(t, 10), HitPoints: creaturetest.HitPoints(t, 40),
			ItemRestrictions: creature.DefaultItemRestrictions(), PlacementRestrictions: creature.DefaultPlacementRestrictions(), Abilities: creature.DefaultAbilities(),
		}
		return match.BattleCombatant{Card: cardtest.CreatureRef(t, id), Spec: spec, HitPoints: creaturetest.HitPoints(t, 40)}
	}
	land := match.BattleLand{Element: board.ElementFire, Level: 1}
	rules := match.BattleRules{AdjacentTerritoryBonus: gamerules.Defaults().Match.AdjacentTerritoryBonus, TerrainBonusPerLevel: gamerules.Defaults().Match.TerrainBonusPerLevel}
	input := match.BattleInput{Attacker: combatant(101), Defender: combatant(201), Land: land, AdjacentTerritories: []board.Position{}, Rules: rules}
	return &BattleInputBuilder{t: t, input: input}
}

func (b *BattleInputBuilder) combatant(side match.BattleSide) *match.BattleCombatant {
	if side == match.BattleSideAttacker {
		return &b.input.Attacker
	}
	return &b.input.Defender
}

// AT は性能の値、HP は戦闘の前の値
func (b *BattleInputBuilder) Stats(side match.BattleSide, attack, hitPoints int) *BattleInputBuilder {
	b.t.Helper()
	combatant := b.combatant(side)
	combatant.Spec.Attack = creaturetest.Attack(b.t, attack)
	combatant.HitPoints = creaturetest.HitPoints(b.t, hitPoints)
	return b
}

func (b *BattleInputBuilder) Elements(side match.BattleSide, values ...string) *BattleInputBuilder {
	b.t.Helper()
	b.combatant(side).Spec.Elements = creaturetest.Elements(b.t, values...)
	return b
}

func (b *BattleInputBuilder) Abilities(side match.BattleSide, abilities ...creature.Ability) *BattleInputBuilder {
	b.t.Helper()
	values, err := creature.NewAbilities(abilities)
	require.NoError(b.t, err)
	b.combatant(side).Spec.Abilities = values
	return b
}

// 出したアイテム（攻める側はカード 301、守る側は 302）
func (b *BattleInputBuilder) Item(side match.BattleSide, category item.Category, attackModifier, hitPointsModifier int) *BattleInputBuilder {
	b.t.Helper()
	id := int64(301)
	if side == match.BattleSideDefender {
		id = 302
	}
	spec := match.ItemSpec{Category: category, AttackModifier: item.NewAttackModifier(attackModifier), HitPointsModifier: item.NewHitPointsModifier(hitPointsModifier)}
	b.combatant(side).Item = &match.BattleItem{Card: cardtest.Ref(b.t, card.ItemEntityType, id), Spec: spec}
	return b
}

func (b *BattleInputBuilder) Land(element board.Element, level int) *BattleInputBuilder {
	b.input.Land.Element, b.input.Land.Level = element, level
	return b
}

// 支援効果に数える隣の領地の数（位置は (0, 0) から x の順）
func (b *BattleInputBuilder) AdjacentTerritories(count int) *BattleInputBuilder {
	b.t.Helper()
	positions := make([]board.Position, count)
	for i := range positions {
		positions[i] = boardtest.Position(b.t, i, 0)
	}
	b.input.AdjacentTerritories = positions
	return b
}

// 支援効果の 1 つあたりと、地形効果のレベルあたりの値
func (b *BattleInputBuilder) Rules(adjacentTerritoryBonus, terrainBonusPerLevel int) *BattleInputBuilder {
	b.t.Helper()
	adjacent, err := gamerules.NewAdjacentTerritoryBonus(adjacentTerritoryBonus)
	require.NoError(b.t, err)
	terrain, err := gamerules.NewTerrainBonusPerLevel(terrainBonusPerLevel)
	require.NoError(b.t, err)
	b.input.Rules.AdjacentTerritoryBonus, b.input.Rules.TerrainBonusPerLevel = adjacent, terrain
	return b
}

func (b *BattleInputBuilder) Input() match.BattleInput {
	return b.input
}
