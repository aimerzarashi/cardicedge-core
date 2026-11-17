package npcmind

import "gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"

// 行動の種類を選ぶときの重み 1 つ。0 以上で、0 はその種類を選ばない
type Weight int

func NewWeight(value int) (Weight, error) {
	if value < 0 {
		return 0, ErrNegativeWeight
	}
	return Weight(value), nil
}

func (w Weight) Int() int {
	return int(w)
}

// 行動の種類ごとの重み（2026-11-17 オーナー。core#187）。対象（手札・領地等）には重みを付けず均等に選ぶ（対象に付けるのは、
// 点を付けて選ぶ強さの工夫に近いため）。無い種類は 0（選ばない）
type Weights map[match.Action]Weight

// 当面の既定値。すべて 1 で、完全ランダムと同じ（値の加減は、マスタで変えられるようになってから見比べる。core#188）
func DefaultWeights() Weights {
	weights := Weights{}
	for _, action := range match.Actions() {
		weights[action] = 1
	}
	return weights
}

// 採点基準（2026-11-17 オーナー。core#190）。値は大きいほどよく、場面の候補の中で 0〜1 にそろえて重みを掛ける
type Criterion string

const (
	// 自分の総魔力 − 相手のうち一番高い総魔力
	CriterionTotalMagicLead Criterion = "total_magic_lead"
	// 自分の領地の数
	CriterionTerritoryCount Criterion = "territory_count"
	// 戦闘の候補だけ、戦闘の後にその領地を自分が持っている割合。戦闘にならない候補は 0
	CriterionBattleHold Criterion = "battle_hold"
	// 手持ちの魔力。当面は盤面の通行料の一番高い額で頭打ち
	CriterionMagicReserve Criterion = "magic_reserve"
)

// 採点基準の並び（計算の順。重みの map の順は不定なので、並びはここで固定する）
func Criteria() []Criterion {
	return []Criterion{CriterionTotalMagicLead, CriterionTerritoryCount, CriterionBattleHold, CriterionMagicReserve}
}

// 採点基準ごとの重み。無い基準は 0（効かない）
type CriterionWeights map[Criterion]Weight

// 当面の既定値。すべて 1（マスタにするのは core#188）
func DefaultCriterionWeights() CriterionWeights {
	weights := CriterionWeights{}
	for _, criterion := range Criteria() {
		weights[criterion] = 1
	}
	return weights
}
