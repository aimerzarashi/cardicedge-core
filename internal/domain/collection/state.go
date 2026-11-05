package collection

import "gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"

type State interface {
	isCollectionState()
}

type UncreatedCollection struct{}

func (UncreatedCollection) isCollectionState() {}

// 持ち主・カード・配布は持たない（作ったことと重複は一意な key が守るため）。足した戦利品の対戦だけは、二度足さないために覚える
// （対戦の数だけ増えるが、1 件は ID 1 つ）。カードの合計を持たないのは、Decide が読まないため
type Collection struct {
	ID              ID
	Version         int64
	RewardedMatches []match.ID
}

func (Collection) isCollectionState() {}

func InitialState() State {
	return UncreatedCollection{}
}
