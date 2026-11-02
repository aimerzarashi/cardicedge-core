package collection

type State interface {
	isCollectionState()
}

type UncreatedCollection struct{}

func (UncreatedCollection) isCollectionState() {}

// 持ち主・カード・配布は持たない（Decide もコマンドも読まず、作ったことと重複は一意な key が守るため）
type Collection struct {
	ID      ID
	Version int64
}

func (Collection) isCollectionState() {}

func InitialState() State {
	return UncreatedCollection{}
}
