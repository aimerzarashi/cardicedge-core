package match

import "gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"

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

// 以下のコマンドは、操作した参加者の番号を持つ。プレイヤーか NPC かは見ず、今の番の人でなければ断る
type DrawCommand struct {
	By ParticipantIndex
}

func (*DrawCommand) isMatchCommand() {}

type DiscardCommand struct {
	By ParticipantIndex
	// 手札の何番目か（0 始まり）
	HandIndex int
}

func (*DiscardCommand) isMatchCommand() {}

type RollCommand struct {
	By ParticipantIndex
}

func (*RollCommand) isMatchCommand() {}

type ChoosePathCommand struct {
	By ParticipantIndex
	To board.Position
}

func (*ChoosePathCommand) isMatchCommand() {}

type EndTurnCommand struct {
	By ParticipantIndex
}

func (*EndTurnCommand) isMatchCommand() {}
