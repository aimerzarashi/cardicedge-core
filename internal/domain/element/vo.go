// カードとボードが共に使う属性の語彙。属性 1 つの型だけを置き、集合の決まり（いくつ持てるか・組み合わせ）は持ち主の側に置く
// （2026-11-15 オーナー。core#178。カードとボードは互いを読まないので、両方が使う語彙をこの段に降ろす）。
// クリーチャーの属性と土地の属性は名前が似るが別の概念なので型を分ける（土地には複があり、比べ方（地形効果）は match の決まり）
package element

import "fmt"

// クリーチャーの属性 1 つ。火・地・水・風と、どれでもない無
type Creature string

const (
	CreatureFire    Creature = "fire"
	CreatureEarth   Creature = "earth"
	CreatureWater   Creature = "water"
	CreatureWind    Creature = "wind"
	CreatureNeutral Creature = "neutral"
)

func NewCreature(value string) (Creature, error) {
	switch creature := Creature(value); creature {
	case CreatureFire, CreatureEarth, CreatureWater, CreatureWind, CreatureNeutral:
		return creature, nil
	}
	return "", fmt.Errorf("%w: %q", ErrCreatureUnknown, value)
}

func (c Creature) String() string {
	return string(c)
}

// 土地の属性 1 つ。火・地・水・風・無・複。複は火・地・水・風の 4 色のマークが描かれた特殊な土地を表す値の 1 つ（組み合わせは持たない）
type Land string

const (
	LandFire    Land = "fire"
	LandEarth   Land = "earth"
	LandWater   Land = "water"
	LandWind    Land = "wind"
	LandNeutral Land = "neutral"
	LandMulti   Land = "multi"
)

func NewLand(value string) (Land, error) {
	switch land := Land(value); land {
	case LandFire, LandEarth, LandWater, LandWind, LandNeutral, LandMulti:
		return land, nil
	}
	return "", fmt.Errorf("%w: %q", ErrLandUnknown, value)
}

func (l Land) String() string {
	return string(l)
}
