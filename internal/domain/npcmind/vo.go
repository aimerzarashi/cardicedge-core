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
