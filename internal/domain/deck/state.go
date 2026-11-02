package deck

type State interface {
	isDeckState()
}

type UncreatedDeck struct{}

func (UncreatedDeck) isDeckState() {}

// 持ち主・名前・カード・配布・番号は持たない（Decide もコマンドも読まず、作ったことと重複は一意な key が守るため）
type Deck struct {
	ID      ID
	Version int64
}

func (Deck) isDeckState() {}

func InitialState() State {
	return UncreatedDeck{}
}
