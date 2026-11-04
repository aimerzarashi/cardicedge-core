package match

type Command interface {
	isMatchCommand()
}

// 対戦を始める。参加者は部屋の席の順。値が使えるか（持っているデッキか等）は部屋で選んだ時に確かめてあり、ここでは見ない
type StartCommand struct {
	ID           ID
	Participants []Participant
	Board        Board
	Rules        Rules
}

func (*StartCommand) isMatchCommand() {}
